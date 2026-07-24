package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// bridge lets a chat/completions tool run on the ChatGPT codex subscription.
//
// tomo (and any OpenAI-dialect tool) speaks POST /v1/chat/completions. The
// codex subscription only answers the Responses wire at chatgpt.com behind an
// OAuth token, and only for the gpt-5.x models that ship with a ChatGPT plan.
// This subcommand stands between the two: it accepts a chat request, translates
// it to a Responses request, forwards it to the codex backend with the token
// from ~/.codex/auth.json, and streams the Responses events back as chat chunks
// the tool already knows how to read.
//
// The point is a fair harness comparison: run tomo and codex on the identical
// model and task, so any difference is the harness, not the model. It drives the
// user's own subscription, which they authorised for this comparison; it is not
// a way to share or resell that access.

// backendDefault is codex's own default ChatGPT backend root; /codex/responses
// hangs off it.
const backendDefault = "https://chatgpt.com/backend-api"

// codexOAuthClientID is the public client id the codex CLI registers with, used
// only to refresh an expired access token against the same endpoint codex uses.
const codexOAuthClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
const codexTokenURL = "https://auth.openai.com/oauth/token"

// bridgeOpts is the resolved configuration for one bridge process.
type bridgeOpts struct {
	port     string
	model    string // overrides the model the tool asks for, when set
	effort   string // reasoning effort: minimal|low|medium|high
	backend  string // Responses backend root
	authPath string // ~/.codex/auth.json
	traceDir string // optional: dump each request/response pair
}

// cmdBridge parses flags and serves until interrupted.
func cmdBridge(ctx context.Context, args []string) error {
	o := bridgeOpts{
		port:     "8790",
		effort:   "high",
		backend:  backendDefault,
		authPath: filepath.Join(os.Getenv("HOME"), ".codex", "auth.json"),
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		next := func() string { i++; return arg(args, i) }
		switch {
		case a == "--port":
			o.port = next()
		case a == "--model":
			o.model = next()
		case a == "--effort":
			o.effort = next()
		case a == "--backend":
			o.backend = next()
		case a == "--auth":
			o.authPath = next()
		case a == "--trace":
			o.traceDir = next()
		case strings.HasPrefix(a, "--port="):
			o.port = strings.TrimPrefix(a, "--port=")
		case strings.HasPrefix(a, "--model="):
			o.model = strings.TrimPrefix(a, "--model=")
		case strings.HasPrefix(a, "--effort="):
			o.effort = strings.TrimPrefix(a, "--effort=")
		}
	}

	br, err := newBridge(o)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", br.serve)
	mux.HandleFunc("/chat/completions", br.serve)
	// Real codex speaks the Responses wire natively and leans on the codex backend
	// to inject its shell and apply_patch tools server-side, so its request carries
	// no tools of its own. Round-tripping that through chat and back would strip the
	// tools it never sent, leaving the model with nothing to run. The passthrough
	// route forwards codex's own Responses request to /codex/responses verbatim, so
	// the backend still injects the tools and codex behaves exactly as it would on
	// its own subscription; only the model and effort are pinned for a fair column.
	// claude-code speaks the Anthropic Messages wire; serveAnthropic translates it
	// to the same Responses request every other tool produces, so a claude run
	// audits and reconstructs identically. Its token-count probe is answered with a
	// cheap estimate so the CLI's context accounting does not stall the run.
	mux.HandleFunc("/v1/messages/count_tokens", br.serveAnthropicCountTokens)
	mux.HandleFunc("/v1/messages", br.serveAnthropic)
	mux.HandleFunc("/messages", br.serveAnthropic)
	mux.HandleFunc("/v1/responses", br.servePassthrough)
	mux.HandleFunc("/responses", br.servePassthrough)
	mux.HandleFunc("/v1/models", br.models)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok\n") })

	srv := &http.Server{Addr: ":" + o.port, Handler: mux}
	go func() {
		<-ctx.Done()
		sc, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(sc)
	}()
	fmt.Fprintf(os.Stderr, "codex bridge: http://localhost:%s/v1  ->  %s/codex/responses  (model=%s effort=%s)\n",
		o.port, o.backend, orDash(o.model), o.effort)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "(from request)"
	}
	return s
}

// bridge holds the live token and forwards requests.
type bridge struct {
	o      bridgeOpts
	client *http.Client
	mu     sync.Mutex
	tok    codexAuth
	seq    int
}

// codexAuth mirrors the tokens block of ~/.codex/auth.json. Values are never
// logged.
type codexAuth struct {
	Tokens struct {
		IDToken      string `json:"id_token"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		AccountID    string `json:"account_id"`
	} `json:"tokens"`
	LastRefresh string `json:"last_refresh"`
}

func newBridge(o bridgeOpts) (*bridge, error) {
	b := &bridge{o: o, client: &http.Client{Timeout: 0}}
	if err := b.loadAuth(); err != nil {
		return nil, err
	}
	if b.tok.Tokens.AccessToken == "" || b.tok.Tokens.AccountID == "" {
		return nil, fmt.Errorf("bridge: %s has no access_token/account_id; run a codex command to sign in first", o.authPath)
	}
	return b, nil
}

func (b *bridge) loadAuth() error {
	data, err := os.ReadFile(b.o.authPath)
	if err != nil {
		return fmt.Errorf("bridge: read %s: %w", b.o.authPath, err)
	}
	var a codexAuth
	if err := json.Unmarshal(data, &a); err != nil {
		return fmt.Errorf("bridge: parse %s: %w", b.o.authPath, err)
	}
	b.mu.Lock()
	b.tok = a
	b.mu.Unlock()
	return nil
}

func (b *bridge) access() (token, account string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tok.Tokens.AccessToken, b.tok.Tokens.AccountID
}

// models answers the discovery endpoint some tools probe on startup.
func (b *bridge) models(w http.ResponseWriter, r *http.Request) {
	m := b.o.model
	if m == "" {
		m = "gpt-5.6-sol"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data":   []any{map[string]any{"id": m, "object": "model", "owned_by": "openai"}},
	})
}

// serve translates one chat request, forwards it to the codex backend, and
// streams the answer back as chat chunks.
func (b *bridge) serve(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	b.mu.Lock()
	b.seq++
	seq := b.seq
	b.mu.Unlock()

	respReq, err := chatRequestToResponses(body, b.o.model, b.o.effort)
	if err != nil {
		http.Error(w, "translate: "+err.Error(), http.StatusBadRequest)
		return
	}
	rb, _ := json.Marshal(respReq)
	if b.o.traceDir != "" {
		b.dump(seq, "req", rb)
	}

	resp, err := b.forward(r.Context(), rb, true)
	if err != nil {
		http.Error(w, "backend: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		if b.o.traceDir != "" {
			b.dump(seq, "err", msg)
		}
		// Surface the backend status verbatim so the tool sees the real failure
		// (a 401 means the token needs a refresh; a 429 is the plan's rate limit).
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(msg)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flush := func() {
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	// Tee the upstream Responses SSE to disk while it streams to the agent, so the
	// assistant's reply for this turn, its text, its reasoning, and its tool calls,
	// is captured. Without this only the request is dumped, and the assistant turn
	// survives only as an echo in the next request's input, which loses the final
	// turn entirely and any prose a tool-only agent never echoes back.
	respBody := b.teeResponse(seq, resp.Body)
	responsesStreamToChat(w, flush, respBody, seq, b.o.model)
}

// teeResponse wraps an upstream response body so every byte streamed to the agent
// is also written to <seq>.resp in the trace dir. It is a no-op passthrough when
// tracing is off. The file is closed when the returned reader is fully drained,
// which the streamer does, so no handle leaks per turn.
func (b *bridge) teeResponse(seq int, upstream io.ReadCloser) io.Reader {
	if b.o.traceDir == "" {
		return upstream
	}
	_ = os.MkdirAll(b.o.traceDir, 0o755)
	f, err := os.Create(filepath.Join(b.o.traceDir, fmt.Sprintf("%04d.resp", seq)))
	if err != nil {
		return upstream
	}
	return &closingTeeReader{r: io.TeeReader(upstream, f), f: f}
}

// closingTeeReader closes its capture file once the underlying stream reports EOF,
// so a per-turn tee needs no separate lifecycle from the read that drains it.
type closingTeeReader struct {
	r    io.Reader
	f    *os.File
	done bool
}

func (t *closingTeeReader) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	if err != nil && !t.done {
		t.done = true
		_ = t.f.Close()
	}
	return n, err
}

// servePassthrough forwards a native Responses request (real codex speaks this
// wire) to the codex backend without translating it to chat and back. Codex's
// request already carries its developer instructions and relies on the backend
// to inject its tools, so the only edits are pinning the model and reasoning
// effort when the bridge was started with them; everything else rides through
// byte for byte. The backend's Responses SSE is streamed straight back, since
// codex consumes that wire natively.
func (b *bridge) servePassthrough(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	b.mu.Lock()
	b.seq++
	seq := b.seq
	b.mu.Unlock()

	// Pin the model and effort so the codex column runs on the chosen gpt-5.x
	// variant at a controlled effort, leaving the rest of codex's request intact.
	// The reasoning effort is merged into codex's own reasoning block rather than
	// replacing it, so fields like context survive.
	out := body
	if b.o.model != "" || b.o.effort != "" {
		var m map[string]any
		if json.Unmarshal(body, &m) == nil {
			if b.o.model != "" {
				m["model"] = b.o.model
			}
			if b.o.effort != "" {
				rz, _ := m["reasoning"].(map[string]any)
				if rz == nil {
					rz = map[string]any{}
				}
				rz["effort"] = b.o.effort
				m["reasoning"] = rz
			}
			if nb, err := json.Marshal(m); err == nil {
				out = nb
			}
		}
	}
	if b.o.traceDir != "" {
		b.dump(seq, "req", out)
	}

	resp, err := b.forward(r.Context(), out, true)
	if err != nil {
		http.Error(w, "backend: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		if b.o.traceDir != "" {
			b.dump(seq, "err", msg)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(msg)
		return
	}

	// Relay the Responses SSE unchanged, flushing on each frame boundary (the blank
	// line between events) so codex sees tokens as they arrive rather than in one
	// lump at the end.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flush := func() {
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	sc := bufio.NewScanner(b.teeResponse(seq, resp.Body))
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		w.Write(line)
		w.Write([]byte("\n"))
		if len(bytes.TrimSpace(line)) == 0 {
			flush()
		}
	}
	flush()
}

// forward posts a Responses request to the codex backend with the subscription
// token. On a 401 it refreshes once and retries.
func (b *bridge) forward(ctx context.Context, body []byte, retry bool) (*http.Response, error) {
	token, account := b.access()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(b.o.backend, "/")+"/codex/responses", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("chatgpt-account-id", account)
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("originator", "codex_cli_rs")
	req.Header.Set("User-Agent", "codex_cli_rs")
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && retry {
		resp.Body.Close()
		if err := b.refresh(ctx); err != nil {
			return nil, fmt.Errorf("token expired and refresh failed: %w (try any codex command to re-auth)", err)
		}
		return b.forward(ctx, body, false)
	}
	return resp, nil
}

// refresh trades the refresh token for a new access token and rewrites
// auth.json, the same exchange codex performs when its token lapses.
func (b *bridge) refresh(ctx context.Context) error {
	b.mu.Lock()
	rt := b.tok.Tokens.RefreshToken
	b.mu.Unlock()
	if rt == "" {
		return fmt.Errorf("no refresh_token in auth.json")
	}
	payload, _ := json.Marshal(map[string]string{
		"client_id":     codexOAuthClientID,
		"grant_type":    "refresh_token",
		"refresh_token": rt,
		"scope":         "openid profile email",
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, codexTokenURL, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token endpoint %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var got struct {
		AccessToken  string `json:"access_token"`
		IDToken      string `json:"id_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		return err
	}
	b.mu.Lock()
	if got.AccessToken != "" {
		b.tok.Tokens.AccessToken = got.AccessToken
	}
	if got.IDToken != "" {
		b.tok.Tokens.IDToken = got.IDToken
	}
	if got.RefreshToken != "" {
		b.tok.Tokens.RefreshToken = got.RefreshToken
	}
	b.tok.LastRefresh = time.Now().UTC().Format(time.RFC3339)
	out, _ := json.MarshalIndent(b.tok, "", "  ")
	b.mu.Unlock()
	// Best effort: keep auth.json current so codex and the bridge stay in sync.
	_ = os.WriteFile(b.o.authPath, out, 0o600)
	return nil
}

func (b *bridge) dump(seq int, kind string, body []byte) {
	_ = os.MkdirAll(b.o.traceDir, 0o755)
	_ = os.WriteFile(filepath.Join(b.o.traceDir, fmt.Sprintf("%04d.%s.json", seq, kind)), body, 0o644)
}

// readSSELines calls fn for each data: payload in an SSE stream.
func readSSELines(r io.Reader, fn func(payload []byte)) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(line[len("data:"):])
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		fn(payload)
	}
}
