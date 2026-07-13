package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRedactHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer sk-secret")
	h.Set("Cookie", "session=abc")
	h.Set("X-Api-Key", "key-123")
	h.Set("Content-Type", "application/json")
	h.Add("X-Request-Id", "req-1")

	out := redactHeaders(h)
	for _, k := range []string{"Authorization", "Cookie", "X-Api-Key"} {
		if got := out[k]; len(got) != 1 || got[0] != "REDACTED" {
			t.Errorf("%s = %v, want [REDACTED]", k, got)
		}
	}
	if got := out["Content-Type"]; len(got) != 1 || got[0] != "application/json" {
		t.Errorf("Content-Type = %v, want it kept", got)
	}
	if got := out["X-Request-Id"]; len(got) != 1 || got[0] != "req-1" {
		t.Errorf("X-Request-Id = %v, want it kept", got)
	}
	if redactHeaders(http.Header{}) != nil {
		t.Error("empty header should redact to nil")
	}
}

func TestEnvBool(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", "yes", "On"} {
		t.Setenv("LAB_TEST_FLAG", v)
		if !envBool("LAB_TEST_FLAG") {
			t.Errorf("envBool(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"", "0", "false", "no", "off", "maybe"} {
		t.Setenv("LAB_TEST_FLAG", v)
		if envBool("LAB_TEST_FLAG") {
			t.Errorf("envBool(%q) = true, want false", v)
		}
	}
}

// In passthrough mode a Responses-API call, which the default mode would
// translate to chat completions, is forwarded to the upstream verbatim: same
// path, same body. The tap records the request and response headers with the
// credential headers redacted, so a real subscription run keeps the routing
// headers without ever writing its token.
func TestPassthroughForwardsVerbatimAndCapturesHeaders(t *testing.T) {
	var gotPath, gotBody, gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("X-Request-Id", "resp-42")
		w.Header().Set("Set-Cookie", "sess=leak")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`))
	}))
	defer upstream.Close()

	addr := freeAddr(t)
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = Run(ctx, Options{Upstream: upstream.URL, Addr: addr, TraceDir: dir, Passthrough: true})
	}()

	reqBody := `{"model":"gpt-5.4-mini","input":"hi"}`
	resp := postUntilUp(t, "http://"+addr+"/v1/responses", reqBody)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// The upstream saw the Responses path and the exact body, not a chat-completions
	// rewrite: that is the whole point of passthrough.
	if gotPath != "/v1/responses" {
		t.Errorf("upstream path = %q, want /v1/responses (not translated)", gotPath)
	}
	if gotBody != reqBody {
		t.Errorf("upstream body = %q, want it forwarded verbatim", gotBody)
	}
	if gotAuth != "Bearer sk-live" {
		t.Errorf("upstream auth = %q, want the real token forwarded through", gotAuth)
	}

	// The recorded request keeps the headers with the token redacted.
	req := lastJSON(t, filepath.Join(dir, "requests.jsonl"))
	hdrs, _ := req["headers"].(map[string]any)
	if hdrs == nil {
		t.Fatal("no headers captured on the request")
	}
	if got := firstHeader(hdrs, "Authorization"); got != "REDACTED" {
		t.Errorf("recorded Authorization = %q, want REDACTED", got)
	}
	if got := firstHeader(hdrs, "Content-Type"); got != "application/json" {
		t.Errorf("recorded Content-Type = %q, want kept", got)
	}

	// The response headers are captured too, with Set-Cookie redacted and the
	// request id kept for correlation.
	rr := lastJSON(t, filepath.Join(dir, "responses.jsonl"))
	rh, _ := rr["headers"].(map[string]any)
	if rh == nil {
		t.Fatal("no response headers captured")
	}
	if got := firstHeader(rh, "Set-Cookie"); got != "REDACTED" {
		t.Errorf("recorded Set-Cookie = %q, want REDACTED", got)
	}
	if got := firstHeader(rh, "X-Request-Id"); got != "resp-42" {
		t.Errorf("recorded X-Request-Id = %q, want resp-42", got)
	}
}

// freeAddr grabs a free localhost port and returns it as host:port. The listener
// is closed right away, so there is a slim race before Run rebinds it, which is
// fine for a test.
func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

// postUntilUp posts the body, retrying until the proxy goroutine has bound the
// port or the deadline passes.
func postUntilUp(t *testing.T, url, body string) *http.Response {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer sk-live")
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			return resp
		}
		if time.Now().After(deadline) {
			t.Fatalf("proxy never came up: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// lastJSON reads the last non-blank line of a JSONL file as a map.
func lastJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var last []byte
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		if len(strings.TrimSpace(sc.Text())) > 0 {
			last = append([]byte(nil), sc.Bytes()...)
		}
	}
	if last == nil {
		t.Fatalf("%s is empty", path)
	}
	var m map[string]any
	if err := json.Unmarshal(last, &m); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return m
}

// firstHeader pulls the first value of a captured header, which JSON decodes as
// a []any of strings.
func firstHeader(h map[string]any, key string) string {
	vals, ok := h[key].([]any)
	if !ok || len(vals) == 0 {
		return ""
	}
	s, _ := vals[0].(string)
	return s
}
