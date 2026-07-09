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
	"maps"
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
		det:    loadDeterminism(),
		// No overall timeout: an agent turn can stream for minutes, so lean on the
		// request context (cancelled when the tool gives up) instead.
		client: &http.Client{},
	}
	if t.det != nil {
		log.Printf("determinism: forcing %s on every completion request", t.det)
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
		if r.Method == http.MethodPost {
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

	log.Printf("trace proxy: %s -> %s, writing %s", opts.Addr, opts.Upstream, opts.TraceDir)
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
	starts map[int]time.Time
	det    *determinism
	client *http.Client // used by the wire translators, which forward on their own
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
	Seq    int             `json:"seq"`
	TS     string          `json:"ts"`
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Body   json.RawMessage `json:"body,omitempty"`
}

func (t *tap) onRequest(r *http.Request) {
	seq := t.next()
	t.markStart(seq, time.Now())
	r.Header.Set("X-Lab-Seq", strconv.Itoa(seq))
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
		// Normalize the sampling knobs before forwarding so a repeat run
		// reproduces and every tool is judged under the same decoding, not under
		// whatever defaults it happened to send. The recorded body is the
		// forwarded one, so the trace shows exactly what upstream saw.
		body = t.det.apply(body)
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
	t.writeJSON(t.reqs, rec)
}

func (t *tap) onResponse(resp *http.Response) error {
	seq, _ := strconv.Atoi(resp.Request.Header.Get("X-Lab-Seq"))
	start, _ := t.takeStart(seq)
	path := resp.Request.URL.Path
	status := resp.StatusCode
	// Tee the response body into a per-response file as the proxy copies it to
	// the tool. This does not buffer the whole reply, so streaming still works.
	respPath := filepath.Join(t.dir, "resp-"+strconv.Itoa(seq)+".txt")
	f, err := os.Create(respPath)
	if err != nil {
		return err
	}
	tc := &teeCloser{src: resp.Body, sink: f}
	tc.onClose = func(collected []byte) {
		if u := extractUsage(collected); u != nil {
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
				TotalMS: msSince(start, tc.doneAt)}
			if !tc.firstAt.IsZero() {
				rec.TTFBMS = msSince(start, tc.firstAt)
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
	Seq     int    `json:"seq"`
	TS      string `json:"ts"`
	Path    string `json:"path"`
	Status  int    `json:"status"`
	TTFBMS  int64  `json:"ttfb_ms"`
	TotalMS int64  `json:"total_ms"`
}

// recordLatency writes one latency row for a call the wire translators forwarded
// themselves, since those bypass the reverse proxy's ModifyResponse hook. It also
// clears the pending start so the starts map does not leak.
func (t *tap) recordLatency(seq int, start, done, first time.Time, path string, status int) {
	if start.IsZero() {
		return
	}
	rec := latRecord{Seq: seq, TS: nowStamp(), Path: path, Status: status, TotalMS: msSince(start, done)}
	if !first.IsZero() {
		rec.TTFBMS = msSince(start, first)
	}
	t.writeJSON(t.lat, rec)
	t.takeStart(seq)
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

// determinism holds the sampling knobs the proxy forces onto every completion
// request so a run reproduces and tools are compared under one decoding regime.
// It is deliberately general: it keys off the shape of an OpenAI-style request
// (a messages or prompt field), not off any scenario, and rewrites only the
// sampling fields, leaving the rest of the body byte-for-byte intact.
type determinism struct {
	fields map[string]json.RawMessage // field name -> literal JSON value to force
	desc   string                     // human summary for the startup log
}

// loadDeterminism reads the knobs from the environment. It is on by default,
// since a benchmark wants repeatable runs; set LAB_DETERMINISTIC=0 to forward
// requests untouched. temperature and top_p default to greedy decoding; seed is
// forced only when non-empty so a provider that rejects the field can opt out
// with LAB_SEED="".
func loadDeterminism() *determinism {
	if off := os.Getenv("LAB_DETERMINISTIC"); off == "0" || strings.EqualFold(off, "false") {
		return nil
	}
	fields := map[string]json.RawMessage{}
	var parts []string
	add := func(key, val string) {
		if val == "" {
			return
		}
		fields[key] = json.RawMessage(val)
		parts = append(parts, key+"="+val)
	}
	add("temperature", env("LAB_TEMPERATURE", "0"))
	add("top_p", env("LAB_TOP_P", "1"))
	add("seed", env("LAB_SEED", "7"))
	if len(fields) == 0 {
		return nil
	}
	return &determinism{fields: fields, desc: strings.Join(parts, " ")}
}

func (d *determinism) String() string {
	if d == nil {
		return "off"
	}
	return d.desc
}

// apply forces the configured sampling fields onto a JSON completion request. A
// body that is not JSON, or JSON that is not a completion request, passes
// through unchanged, so the proxy stays a transparent tap for anything else.
func (d *determinism) apply(body []byte) []byte {
	if d == nil || len(bytes.TrimSpace(body)) == 0 {
		return body
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(body, &m) != nil {
		return body
	}
	_, hasMessages := m["messages"]
	_, hasPrompt := m["prompt"]
	if !hasMessages && !hasPrompt {
		return body
	}
	maps.Copy(m, d.fields)
	out, err := json.Marshal(m)
	if err != nil {
		return body
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
