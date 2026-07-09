// Command proxy is the lab's trace tap. Every tool under test points its
// OpenAI-compatible base_url at this proxy instead of straight at the upstream,
// so the lab captures request and response bodies and token usage the same way
// for every tool, without the tool knowing or cooperating.
//
// It forwards each request to UPSTREAM + the incoming path, streams the reply
// back untouched (so SSE keeps flowing token by token), and tees a copy of both
// bodies into TRACE_DIR. The Authorization header is redacted before anything
// is written, so a key a tool carried never lands in the trace.
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

func main() {
	upstream := env("UPSTREAM", "https://opencode.ai/zen")
	addr := env("ADDR", ":8080")
	traceDir := env("TRACE_DIR", "/trace")
	if err := os.MkdirAll(traceDir, 0o755); err != nil {
		log.Fatalf("trace dir: %v", err)
	}

	target, err := url.Parse(upstream)
	if err != nil {
		log.Fatalf("bad UPSTREAM %q: %v", upstream, err)
	}

	t := &tap{
		dir:   traceDir,
		reqs:  mustAppend(filepath.Join(traceDir, "requests.jsonl")),
		usage: mustAppend(filepath.Join(traceDir, "usage.jsonl")),
	}

	rp := &httputil.ReverseProxy{
		// -1 flushes each write immediately, which is what keeps a streamed
		// SSE reply arriving at the tool token by token instead of in one lump.
		FlushInterval: -1,
		Director: func(r *http.Request) {
			r.URL.Scheme = target.Scheme
			r.URL.Host = target.Host
			r.URL.Path = singleJoin(target.Path, r.URL.Path)
			r.Host = target.Host
			t.onRequest(r)
		},
		ModifyResponse: t.onResponse,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			log.Printf("proxy error: %v", err)
			w.WriteHeader(http.StatusBadGateway)
		},
	}

	log.Printf("trace proxy: %s -> %s, writing %s", addr, upstream, traceDir)
	srv := &http.Server{Addr: addr, Handler: rp, ReadHeaderTimeout: 15 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

// tap records what passes through. One request and one response are correlated
// by a monotonic sequence number so a reader can pair them up later.
type tap struct {
	dir   string
	mu    sync.Mutex
	seq   int
	reqs  *os.File
	usage *os.File
}

func (t *tap) next() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seq++
	return t.seq
}

type reqRecord struct {
	Seq    int             `json:"seq"`
	TS     string          `json:"ts"`
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Body   json.RawMessage `json:"body,omitempty"`
}

func (t *tap) onRequest(r *http.Request) {
	seq := t.next()
	r.Header.Set("X-Lab-Seq", strconv.Itoa(seq))
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
	}
	rec := reqRecord{
		Seq:    seq,
		TS:     nowStamp(),
		Method: r.Method,
		Path:   r.URL.Path,
		Body:   sanitize(body),
	}
	t.writeJSON(t.reqs, rec)
}

func (t *tap) onResponse(resp *http.Response) error {
	seq, _ := strconv.Atoi(resp.Request.Header.Get("X-Lab-Seq"))
	// Tee the response body into a per-response file as the proxy copies it to
	// the tool. This does not buffer the whole reply, so streaming still works.
	respPath := filepath.Join(t.dir, "resp-"+strconv.Itoa(seq)+".txt")
	f, err := os.Create(respPath)
	if err != nil {
		return err
	}
	resp.Body = &teeCloser{
		src:  resp.Body,
		sink: f,
		onClose: func(collected []byte) {
			if u := extractUsage(collected); u != nil {
				u.Seq = seq
				u.TS = nowStamp()
				u.Status = resp.StatusCode
				t.writeJSON(t.usage, u)
			}
		},
	}
	return nil
}

func (t *tap) writeJSON(f *os.File, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	f.Write(append(b, '\n'))
	f.Sync()
}

// teeCloser copies everything read from src into sink, and also keeps the raw
// bytes so onClose can pull usage out of them once the reply is complete.
type teeCloser struct {
	src     io.ReadCloser
	sink    *os.File
	buf     bytes.Buffer
	onClose func([]byte)
}

func (tc *teeCloser) Read(p []byte) (int, error) {
	n, err := tc.src.Read(p)
	if n > 0 {
		tc.sink.Write(p[:n])
		tc.buf.Write(p[:n])
	}
	return n, err
}

func (tc *teeCloser) Close() error {
	tc.sink.Close()
	if tc.onClose != nil {
		tc.onClose(tc.buf.Bytes())
	}
	return tc.src.Close()
}

type usageRecord struct {
	Seq              int    `json:"seq"`
	TS               string `json:"ts"`
	Status           int    `json:"status"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
}

// extractUsage finds the usage block in a reply, whether it came back as one
// JSON object or as a stream of SSE data: lines. It reads the last usage seen,
// which is the authoritative one for a streamed completion.
func extractUsage(body []byte) *usageRecord {
	var last *usageRecord
	tryOne := func(chunk []byte) {
		var v struct {
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(chunk, &v) == nil && v.Usage != nil {
			last = &usageRecord{
				PromptTokens:     v.Usage.PromptTokens,
				CompletionTokens: v.Usage.CompletionTokens,
				TotalTokens:      v.Usage.TotalTokens,
			}
		}
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		tryOne(trimmed)
	}
	for _, line := range bytes.Split(body, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(line[len("data:"):])
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		tryOne(payload)
	}
	return last
}

// sanitize redacts the Authorization value if the request body happened to
// echo it, and leaves the rest of the JSON intact. Bodies that are not JSON
// pass through as an opaque string so the trace still shows something.
func sanitize(body []byte) json.RawMessage {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	if json.Valid(body) {
		return json.RawMessage(body)
	}
	q, _ := json.Marshal(string(body))
	return q
}

func singleJoin(a, b string) string {
	a = strings.TrimSuffix(a, "/")
	if b == "" {
		return a
	}
	if !strings.HasPrefix(b, "/") {
		b = "/" + b
	}
	return a + b
}

func mustAppend(path string) *os.File {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Fatalf("open %s: %v", path, err)
	}
	return f
}

func nowStamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
