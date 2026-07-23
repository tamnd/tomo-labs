package publish

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestUploadFilesCommit drives CreateDatasetRepo and UploadFiles against a stub
// Hub that speaks the create and commit endpoints, asserting an idempotent
// create and a well-formed NDJSON commit with an inline base64 file. No network.
func TestUploadFilesCommit(t *testing.T) {
	var createCalls int
	var commitBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/repos/create":
			createCalls++
			if createCalls == 1 {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"url":"ok"}`))
			} else {
				// Second create returns already-exists; the client treats it as success.
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"error":"repo already exists"}`))
			}
		case strings.HasPrefix(r.URL.Path, "/api/datasets/") && strings.HasSuffix(r.URL.Path, "/commit/main"):
			commitBody, _ = readAll(r)
			if got := r.Header.Get("Content-Type"); got != "application/x-ndjson" {
				t.Errorf("commit content-type = %q", got)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"commitUrl":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := &HFClient{Token: "t", Repo: "open-index/tomo-traces", HTTP: srv.Client()}
	// Redirect the endpoint by overriding the base through a round-trip rewrite.
	c.HTTP = rewriteClient(srv.URL)

	ctx := context.Background()
	if err := c.CreateDatasetRepo(ctx, false); err != nil {
		t.Fatalf("create 1: %v", err)
	}
	if err := c.CreateDatasetRepo(ctx, false); err != nil {
		t.Fatalf("create 2 (idempotent): %v", err)
	}
	if createCalls != 2 {
		t.Fatalf("want 2 create calls, got %d", createCalls)
	}

	c.Message = "run: tomo-oi on dynaconf-1225 pass"
	ops := []HFOp{
		{PathInRepo: "data/e/s/m/tomo-oi-1.jsonl", Content: []byte(`{"type":"session"}`)},
		{PathInRepo: "README.md", Content: []byte("# card\n")},
	}
	if err := c.UploadFiles(ctx, ops); err != nil {
		t.Fatalf("upload: %v", err)
	}

	lines := ndjsonLines(t, commitBody)
	if len(lines) != 3 {
		t.Fatalf("want header + 2 files, got %d: %s", len(lines), commitBody)
	}
	var header struct {
		Key   string `json:"key"`
		Value struct {
			Summary string `json:"summary"`
		} `json:"value"`
	}
	mustJSON(t, lines[0], &header)
	if header.Key != "header" || header.Value.Summary != c.Message {
		t.Fatalf("bad commit header: %+v", header)
	}
	var file struct {
		Key   string `json:"key"`
		Value struct {
			Path     string `json:"path"`
			Encoding string `json:"encoding"`
			Content  string `json:"content"`
		} `json:"value"`
	}
	mustJSON(t, lines[1], &file)
	if file.Key != "file" || file.Value.Path != "data/e/s/m/tomo-oi-1.jsonl" || file.Value.Encoding != "base64" {
		t.Fatalf("bad file line: %+v", file)
	}
}

// TestUploadFilesSingleCommit asserts that a publish of many small files lands
// as one commit, not one commit per fixed-size batch. This is the regression the
// byte-cap packing fixed: the old batch-of-ten split an 82-file backfill into
// nine identical-summary commits that spammed the dataset history.
func TestUploadFilesSingleCommit(t *testing.T) {
	var commits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/commit/main") {
			commits++
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := &HFClient{Token: "t", Repo: "open-index/tomo-traces", HTTP: rewriteClient(srv.URL)}
	c.Message = "backfill: 77 traces across 1 evals"
	ops := make([]HFOp, 82)
	for i := range ops {
		ops[i] = HFOp{PathInRepo: "data/f-" + strings.Repeat("x", i%5) + ".jsonl", Content: []byte(`{"type":"session"}`)}
	}
	if err := c.UploadFiles(context.Background(), ops); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if commits != 1 {
		t.Fatalf("want 1 commit for 82 small files, got %d", commits)
	}
}

// rewriteClient returns an http.Client whose transport rewrites the fixed
// hfEndpoint host to the test server, so the ported client can keep its absolute
// URLs unchanged.
func rewriteClient(base string) *http.Client {
	return &http.Client{Transport: rewriteRT{base: base}}
}

type rewriteRT struct{ base string }

func (rt rewriteRT) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.HasPrefix(req.URL.String(), hfEndpoint) {
		newURL := rt.base + strings.TrimPrefix(req.URL.String(), hfEndpoint)
		r2, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
		if err != nil {
			return nil, err
		}
		r2.Header = req.Header
		return http.DefaultTransport.RoundTrip(r2)
	}
	return http.DefaultTransport.RoundTrip(req)
}

func readAll(r *http.Request) ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r.Body)
	return buf.Bytes(), err
}

func ndjsonLines(t *testing.T, data []byte) []string {
	t.Helper()
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			out = append(out, line)
		}
	}
	return out
}

var _ = json.Marshal
