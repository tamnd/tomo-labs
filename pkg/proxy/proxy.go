// Package proxy is the lab's trace tap. Every tool under test points its
// OpenAI-compatible base_url at this proxy instead of straight at the upstream,
// so the lab captures request and response bodies and token usage the same way
// for every tool, without the tool knowing or cooperating.
//
// It forwards each request to the upstream plus the incoming path, streams the
// reply back untouched (so SSE keeps flowing token by token), and tees a copy of
// both bodies into the trace dir. The Authorization header is redacted before
// anything is written, so a key a tool carried never lands in the trace.
//
// The logic lives in this package so it can be embedded and tested directly; the
// cmd/proxy binary is a thin main that calls Run.
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

	"github.com/tamnd/tomo/pkg/wire"
)

// Options configures a proxy server. Zero values fall back to the environment
// and then to sensible defaults, so the binary can call Run with a zero Options.
type Options struct {
	Upstream string // default $UPSTREAM or https://opencode.ai/zen
	Addr     string // default $ADDR or :8080
	TraceDir string // default $TRACE_DIR or /trace

	// Passthrough turns the proxy into a raw tap that never rewrites a request.
	// The default mode translates foreign wire calls (OpenAI Responses, Anthropic
	// Messages, Google Gemini) to chat completions so every tool runs on the one
	// shared free model. That translation is wrong for a tool running on its own
	// real subscription: codex talks the Responses wire to its own gpt-5.x backend,
	// and we want the exact bytes it sent and got back, not a rewrite. With
	// Passthrough set the wire switch is skipped, so every request forwards to the
	// upstream verbatim, and the tap also records request and response headers (the
	// model, org, and request id live there) with the credential headers redacted.
	// Set by Options or $PASSTHROUGH.
	Passthrough bool
}

// Run starts the proxy and blocks until the context is cancelled or the server
// stops. It is the whole binary; it is exported so tests and embedders can run a
// real tap against a fake upstream.
func Run(ctx context.Context, opts Options) error {
	if opts.Upstream == "" {
		opts.Upstream = env("UPSTREAM", "https://opencode.ai/zen")
	}
	if opts.Addr == "" {
		opts.Addr = env("ADDR", ":8080")
	}
	if opts.TraceDir == "" {
		opts.TraceDir = env("TRACE_DIR", "/trace")
	}
	if !opts.Passthrough && envBool("PASSTHROUGH") {
		opts.Passthrough = true
	}
	if err := os.MkdirAll(opts.TraceDir, 0o755); err != nil {
		return err
	}
	target, err := url.Parse(opts.Upstream)
	if err != nil {
		return err
	}

	t := &tap{
		dir:    opts.TraceDir,
		reqs:   mustAppend(filepath.Join(opts.TraceDir, "requests.jsonl")),
		usage:  mustAppend(filepath.Join(opts.TraceDir, "usage.jsonl")),
		lat:    mustAppend(filepath.Join(opts.TraceDir, "latency.jsonl")),
		starts: map[int]time.Time{},
		// No overall timeout: an agent turn can stream for minutes, so lean on the
		// request context (cancelled when the tool gives up) instead.
		client: &http.Client{},
		// In passthrough mode record headers too, so a real-subscription run keeps
		// the model, org, and request id that only ride in the headers. The
		// benchmark mode leaves them off to keep its traces to just the bodies.
		captureHeaders: opts.Passthrough,
	}
	if t.captureHeaders {
		t.resp = mustAppend(filepath.Join(opts.TraceDir, "responses.jsonl"))
	}

	rp := &httputil.ReverseProxy{
		// -1 flushes each write immediately, which is what keeps a streamed SSE
		// reply arriving at the tool token by token instead of in one lump.
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

	// Most traffic passes straight through the reverse proxy. The foreign-wire
	// calls are the exception: the upstream speaks chat completions, so an OpenAI
	// Responses, an Anthropic Messages, or a Google Gemini request gets translated
	// to chat and back rather than forwarded verbatim. That way a tool speaks its
	// native wire and still runs on the one shared model every tool is graded on.
	// The translation itself lives in tomo's pkg/wire; here we only orchestrate the
	// forward and the trace capture.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Passthrough skips the wire switch entirely: a real-subscription run keeps
		// talking its own wire to its own backend, so the tap forwards it verbatim
		// and only watches. The default benchmark mode still translates.
		if !opts.Passthrough && r.Method == http.MethodPost {
			switch {
			case wire.IsResponsesPath(r.URL.Path):
				t.serveResponses(w, r, target)
				return
			case wire.IsMessagesPath(r.URL.Path):
				t.serveMessages(w, r, target)
				return
			}
			if model, stream, ok := wire.IsGeminiPath(r.URL.Path); ok {
				t.serveGemini(w, r, target, model, stream)
				return
			}
		}
		rp.ServeHTTP(w, r)
	})

	mode := "translate"
	if opts.Passthrough {
		mode = "passthrough"
	}
	log.Printf("trace proxy (%s): %s -> %s, writing %s", mode, opts.Addr, opts.Upstream, opts.TraceDir)
	srv := &http.Server{Addr: opts.Addr, Handler: handler, ReadHeaderTimeout: 15 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// tap records what passes through. One request and one response are correlated
// by a monotonic sequence number so a reader can pair them up later.
type tap struct {
	dir    string
	mu     sync.Mutex
	seq    int
	reqs   *os.File
	usage  *os.File
	lat    *os.File
	resp   *os.File // response header log, only opened in passthrough capture mode
	starts map[int]time.Time
	client *http.Client // used by the wire translators, which forward on their own

	// captureHeaders records request and response headers alongside the bodies, so
	// a real-subscription run keeps the model and request id that ride only in the
	// headers. Off by default to keep benchmark traces to the bodies alone.
	captureHeaders bool
}

func (t *tap) next() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seq++
	return t.seq
}

func (t *tap) markStart(seq int, at time.Time) {
	t.mu.Lock()
	t.starts[seq] = at
	t.mu.Unlock()
}

func (t *tap) takeStart(seq int) (time.Time, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	at, ok := t.starts[seq]
	delete(t.starts, seq)
	return at, ok
}

type reqRecord struct {
	Seq     int                 `json:"seq"`
	TS      string              `json:"ts"`
	Method  string              `json:"method"`
	Path    string              `json:"path"`
	Headers map[string][]string `json:"headers,omitempty"`
	Body    json.RawMessage     `json:"body,omitempty"`
}

func (t *tap) onRequest(r *http.Request) {
	seq := t.next()
	t.markStart(seq, time.Now())
	r.Header.Set("X-Lab-Seq", strconv.Itoa(seq))
	var body []byte
	if r.Body != nil {
		// Read the body so it can be recorded, then hand the same bytes back for
		// forwarding. The proxy is a transparent tap: it never rewrites the
		// request, so every tool reaches the upstream under the exact sampling it
		// shipped with, and the recorded body is byte-for-byte what upstream saw.
		body, _ = io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		r.Header.Set("Content-Length", strconv.Itoa(len(body)))
	}
	rec := reqRecord{
		Seq:    seq,
		TS:     nowStamp(),
		Method: r.Method,
		Path:   r.URL.Path,
		Body:   sanitize(body),
	}
	if t.captureHeaders {
		rec.Headers = redactHeaders(r.Header)
	}
	t.writeJSON(t.reqs, rec)
}

func (t *tap) onResponse(resp *http.Response) error {
	seq, _ := strconv.Atoi(resp.Request.Header.Get("X-Lab-Seq"))
	start, _ := t.takeStart(seq)
	path := resp.Request.URL.Path
	status := resp.StatusCode
	// A 429 from the free tier carries how long to back off; capture it while the
	// header is still in scope, since the body tee below only sees the body.
	retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
	// In passthrough capture mode record the response headers too, correlated by
	// seq, so a real run keeps the model, request id, and rate-limit headers the
	// body never carries. The body itself is teed raw below as before.
	if t.captureHeaders {
		t.writeJSON(t.resp, respRecord{Seq: seq, TS: nowStamp(), Status: status,
			Headers: redactHeaders(resp.Header)})
	}
	// Tee the response body into a per-response file as the proxy copies it to
	// the tool. This does not buffer the whole reply, so streaming still works.
	respPath := filepath.Join(t.dir, "resp-"+strconv.Itoa(seq)+".txt")
	f, err := os.Create(respPath)
	if err != nil {
		return err
	}
	tc := &teeCloser{src: resp.Body, sink: f}
	tc.onClose = func(collected []byte) {
		u := extractUsage(collected)
		if u != nil {
			u.Seq = seq
			u.TS = nowStamp()
			u.Status = status
			t.writeJSON(t.usage, u)
		}
		// One latency row per response, whatever the status. ttfb is the wait
		// until the first byte came back (dominated by the model thinking), and
		// total spans the whole reply, so a slow stream shows up as total-ttfb.
		if !start.IsZero() {
			rec := latRecord{Seq: seq, TS: nowStamp(), Path: path, Status: status,
				TotalMS: msSince(start, tc.doneAt), RetryAfter: retryAfter}
			if !tc.firstAt.IsZero() {
				rec.TTFBMS = msSince(start, tc.firstAt)
			}
			// A 200 completion that broke mid-stream is an upstream fault, not the
			// agent's; flag it so the harness does not score it as a real failure.
			if status == http.StatusOK && isModelPath(path) {
				if bad, msg := streamFailed(collected, u != nil); bad {
					rec.StreamErr, rec.StreamErrMsg = true, msg
				}
			}
			t.writeJSON(t.lat, rec)
		}
	}
	resp.Body = tc
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
// bytes so onClose can pull usage out of them once the reply is complete. It
// stamps the first byte and the close so the proxy can report latency.
type teeCloser struct {
	src     io.ReadCloser
	sink    *os.File
	buf     bytes.Buffer
	onClose func([]byte)
	firstAt time.Time
	doneAt  time.Time
}

func (tc *teeCloser) Read(p []byte) (int, error) {
	n, err := tc.src.Read(p)
	if n > 0 {
		if tc.firstAt.IsZero() {
			tc.firstAt = time.Now()
		}
		tc.sink.Write(p[:n])
		tc.buf.Write(p[:n])
	}
	return n, err
}

func (tc *teeCloser) Close() error {
	tc.doneAt = time.Now()
	tc.sink.Close()
	if tc.onClose != nil {
		tc.onClose(tc.buf.Bytes())
	}
	return tc.src.Close()
}

type latRecord struct {
	Seq        int    `json:"seq"`
	TS         string `json:"ts"`
	Path       string `json:"path"`
	Status     int    `json:"status"`
	TTFBMS     int64  `json:"ttfb_ms"`
	TotalMS    int64  `json:"total_ms"`
	RetryAfter int    `json:"retry_after_s,omitempty"`
	// StreamErr marks a model call that returned HTTP 200 and then failed
	// mid-stream: the upstream sent an error payload as an SSE data line, or the
	// stream was cut off before its terminating [DONE] and usage. Such a call
	// looks like an empty success in the tokens, so recording it lets the harness
	// tell an upstream fault apart from a real agent failure. StreamErrMsg carries
	// the upstream error text when it sent one.
	StreamErr    bool   `json:"stream_err,omitempty"`
	StreamErrMsg string `json:"stream_err_msg,omitempty"`
}

// recordLatency writes one latency row for a call the wire translators forwarded
// themselves, since those bypass the reverse proxy's ModifyResponse hook. It also
// clears the pending start so the starts map does not leak. retryAfter is the
// upstream's Retry-After header in seconds, or 0 when the call did not carry one.
// streamErr marks a 200 completion that came back with no usage, the fingerprint
// of a stream the upstream dropped mid-flight, so the harness can discount it.
func (t *tap) recordLatency(seq int, start, done, first time.Time, path string, status, retryAfter int, streamErr bool) {
	if start.IsZero() {
		return
	}
	rec := latRecord{Seq: seq, TS: nowStamp(), Path: path, Status: status,
		TotalMS: msSince(start, done), RetryAfter: retryAfter, StreamErr: streamErr}
	if streamErr {
		rec.StreamErrMsg = "truncated stream"
	}
	if !first.IsZero() {
		rec.TTFBMS = msSince(start, first)
	}
	t.writeJSON(t.lat, rec)
	t.takeStart(seq)
}

// parseRetryAfter reads a Retry-After header down to whole seconds. The header is
// either a delay in seconds or an HTTP-date; zen sends delay-seconds, so that is
// tried first, with the date form as a fallback for any upstream that uses it. An
// empty or unparseable header reports 0, which the caller treats as "not given"
// via omitempty.
func parseRetryAfter(h string) int {
	if h == "" {
		return 0
	}
	if n, err := strconv.Atoi(strings.TrimSpace(h)); err == nil && n >= 0 {
		return n
	}
	if t, err := http.ParseTime(h); err == nil {
		if d := time.Until(t); d > 0 {
			return int(d.Seconds())
		}
	}
	return 0
}

// msSince reports whole milliseconds between two instants, guarding the case
// where the end was never stamped (returns 0 rather than a negative number).
func msSince(start, end time.Time) int64 {
	if end.IsZero() || end.Before(start) {
		return 0
	}
	return end.Sub(start).Milliseconds()
}

type usageRecord struct {
	Seq              int     `json:"seq"`
	TS               string  `json:"ts"`
	Status           int     `json:"status"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	CachedTokens     int     `json:"cached_tokens,omitempty"`
	CacheWriteTokens int     `json:"cache_write_tokens,omitempty"`
	CostUSD          float64 `json:"cost_usd,omitempty"`
}

// extractUsage finds the usage block in a reply, whether it came back as one
// JSON object or as a stream of SSE data: lines. It reads the last usage seen,
// which is the authoritative one for a streamed completion.
//
// Beyond the plain token counts it pulls the two things a caller actually pays
// on when the provider reports them: how many prompt tokens were served from a
// cache (cheap) or written to one (a one-time surcharge), and the dollar cost of
// the call. Providers spell these differently, so it reads both the OpenAI shape
// (usage.prompt_tokens_details.cached_tokens, usage.cost) and the Anthropic shape
// (cache_read_input_tokens, cache_creation_input_tokens), and leaves a field zero
// when the provider is silent rather than inventing a number.
func extractUsage(body []byte) *usageRecord {
	var last *usageRecord
	tryOne := func(chunk []byte) {
		var v struct {
			Usage *struct {
				PromptTokens        int     `json:"prompt_tokens"`
				CompletionTokens    int     `json:"completion_tokens"`
				TotalTokens         int     `json:"total_tokens"`
				Cost                float64 `json:"cost"`
				TotalCost           float64 `json:"total_cost"`
				CacheReadInputToks  int     `json:"cache_read_input_tokens"`
				CacheCreateInputTok int     `json:"cache_creation_input_tokens"`
				PromptCacheHitToks  int     `json:"prompt_cache_hit_tokens"`
				PromptTokensDetails *struct {
					CachedTokens int `json:"cached_tokens"`
				} `json:"prompt_tokens_details"`
			} `json:"usage"`
		}
		if json.Unmarshal(chunk, &v) == nil && v.Usage != nil {
			u := v.Usage
			rec := &usageRecord{
				PromptTokens:     u.PromptTokens,
				CompletionTokens: u.CompletionTokens,
				TotalTokens:      u.TotalTokens,
				CacheWriteTokens: u.CacheCreateInputTok,
				CostUSD:          u.Cost,
			}
			if rec.CostUSD == 0 {
				rec.CostUSD = u.TotalCost
			}
			// cached prompt tokens: OpenAI nests them, Anthropic names them flat,
			// DeepSeek reports the hit count under its own name.
			if u.PromptTokensDetails != nil {
				rec.CachedTokens = u.PromptTokensDetails.CachedTokens
			}
			if u.CacheReadInputToks > 0 {
				rec.CachedTokens = u.CacheReadInputToks
			}
			if u.PromptCacheHitToks > 0 {
				rec.CachedTokens = u.PromptCacheHitToks
			}
			last = rec
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

// isModelPath reports whether a path is a model completion endpoint, the only
// place a mid-stream failure is meaningful. It matches the OpenAI chat path and
// the Anthropic messages path, the two the upstream and the wire shims speak.
func isModelPath(p string) bool {
	return strings.Contains(p, "chat/completions") || strings.Contains(p, "/messages")
}

// streamFailed reports whether a model reply came back broken. A gateway that
// drops a completion still answers HTTP 200 and then fails inside the body one
// of two ways: it emits an error object as an SSE data line, or it cuts the
// stream off before the terminating [DONE] and the usage block. Either way the
// tokens look like an empty success, so the caller flags the call as an upstream
// fault instead. hasUsage says whether a usage block was recovered, which a
// complete stream always carries; a stream missing both [DONE] and usage was
// truncated. A non-streamed JSON reply has no data lines and is never flagged
// here, since a 200 object reply is already complete. The returned string is the
// upstream error text when it sent one, for the trace.
func streamFailed(body []byte, hasUsage bool) (bool, string) {
	var errMsg string
	var sawData, sawDone bool
	for _, line := range bytes.Split(body, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(line[len("data:"):])
		if len(payload) == 0 {
			continue
		}
		if bytes.Equal(payload, []byte("[DONE]")) {
			sawDone = true
			continue
		}
		sawData = true
		var v struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(payload, &v) == nil && v.Error != nil {
			errMsg = strings.TrimSpace(v.Error.Message)
		}
	}
	if errMsg != "" {
		return true, errMsg
	}
	if sawData && !sawDone && !hasUsage {
		return true, "truncated stream"
	}
	return false, ""
}

// respRecord is one response's headers, correlated to its request by seq. Only
// written in passthrough capture mode; the body is teed to resp-<seq>.txt as
// usual, so this carries just the status and the redacted headers.
type respRecord struct {
	Seq     int                 `json:"seq"`
	TS      string              `json:"ts"`
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers,omitempty"`
}

// sensitiveHeaders are the header names whose values are credentials or session
// state, redacted before any header is written so a real subscription's token
// never lands in a trace. Keys are the canonical form http.Header uses.
var sensitiveHeaders = map[string]bool{
	"Authorization":       true,
	"Proxy-Authorization": true,
	"Api-Key":             true,
	"X-Api-Key":           true,
	"Openai-Api-Key":      true,
	"Cookie":              true,
	"Set-Cookie":          true,
}

// redactHeaders copies a header map with the credential-bearing values replaced
// by REDACTED, so the trace keeps the useful routing headers (model, org,
// request id, rate limits) without ever recording a key or a session cookie.
func redactHeaders(h http.Header) map[string][]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string][]string, len(h))
	for k, v := range h {
		if sensitiveHeaders[k] {
			out[k] = []string{"REDACTED"}
			continue
		}
		vv := make([]string, len(v))
		copy(vv, v)
		out[k] = vv
	}
	return out
}

// sanitize redacts the Authorization value if the request body happened to echo
// it, and leaves the rest of the JSON intact. Bodies that are not JSON pass
// through as an opaque string so the trace still shows something.
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

// envBool reads a boolean-ish environment flag: 1, true, yes, or on (any case)
// count as set, everything else as unset.
func envBool(k string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(k))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
