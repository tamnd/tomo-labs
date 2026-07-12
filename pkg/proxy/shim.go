// This file lets the proxy speak a foreign LLM wire at its edge while the
// upstream stays chat-only. Our shared free model (deepseek) is served by zen
// over /v1/chat/completions, but codex speaks /v1/responses, claude-code speaks
// /v1/messages, and gemini-cli speaks /v1beta/models/{model}:generateContent.
// Without a shim those tools could not run against the same model every other
// tool uses.
//
// The wire translation itself is pure and lives in tomo's pkg/wire, so it can be
// unit tested and reused. What stays here is the orchestration a tap owns: read
// the request, force the shared decoding knobs, forward to the same upstream
// every tool hits with the caller's key, record the request/usage/latency trace,
// and stream or return the translated reply. Every foreign wire runs through the
// same serveWire path, so codex, claude-code, and gemini-cli all land in the
// metrics on equal footing with a plainly proxied tool.
package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/tamnd/tomo/pkg/wire"
)

// chatCompletionsPath is where the upstream serves the shared model. Gemini's
// request path names the model instead of a chat sibling, so its forward target
// is fixed here rather than derived from the incoming path.
const chatCompletionsPath = "/v1/chat/completions"

// wireCall is the per-wire behavior serveWire needs: how to translate the
// request into chat, how to translate a non-streamed reply back, and how to
// translate the streamed reply back. Everything else (forwarding, auth, trace
// capture) is identical across wires and lives in serveWire.
type wireCall struct {
	// chatPath is the upstream chat-completions path to forward to.
	chatPath string
	// label tags the recorded request so a trace shows which wire it came from.
	label string
	// toChat converts the incoming body to a chat body and reports whether the
	// caller asked to stream.
	toChat func([]byte) ([]byte, bool, error)
	// toJSON converts one non-streamed chat completion back into the wire's shape.
	toJSON func(chat []byte, seq int) map[string]any
	// toStream re-emits the upstream chat SSE stream in the wire's shape, calling
	// onFirst when the first byte arrives and flush after each write, and returns
	// the raw chat usage block so the tap can record tokens.
	toStream func(w io.Writer, flush func(), r io.Reader, seq int, onFirst func()) json.RawMessage
}

// serveResponses drives one OpenAI Responses call end to end.
func (t *tap) serveResponses(w http.ResponseWriter, r *http.Request, target *url.URL) {
	t.serveWire(w, r, target, wireCall{
		chatPath: wire.ChatPathOf(r.URL.Path),
		label:    "(from responses)",
		toChat:   wire.ResponsesToChat,
		toJSON:   wire.ChatToResponses,
		toStream: wire.StreamResponses,
	})
}

// serveMessages drives one Anthropic Messages call end to end.
func (t *tap) serveMessages(w http.ResponseWriter, r *http.Request, target *url.URL) {
	t.serveWire(w, r, target, wireCall{
		chatPath: wire.ChatPathOf(r.URL.Path),
		label:    "(from messages)",
		toChat:   wire.MessagesToChat,
		toJSON:   wire.ChatToMessages,
		toStream: wire.StreamMessages,
	})
}

// serveGemini drives one Google Gemini generateContent call end to end. Gemini
// carries the model in the URL and decides streaming by the method name, so both
// are read from the path by the dispatcher and threaded in here; the chat
// translators do not take a seq for Gemini, so the adapters drop it.
func (t *tap) serveGemini(w http.ResponseWriter, r *http.Request, target *url.URL, model string, stream bool) {
	t.serveWire(w, r, target, wireCall{
		chatPath: chatCompletionsPath,
		label:    "(from gemini)",
		toChat: func(b []byte) ([]byte, bool, error) {
			chat, err := wire.GeminiToChat(b, model, stream)
			return chat, stream, err
		},
		toJSON: func(chat []byte, _ int) map[string]any { return wire.ChatToGemini(chat) },
		toStream: func(w io.Writer, flush func(), r io.Reader, _ int, onFirst func()) json.RawMessage {
			return wire.StreamGemini(w, flush, r, onFirst)
		},
	})
}

// serveWire is the shared orchestration for every translated wire: read the
// request, translate to chat, force the shared decoding knobs, forward upstream
// with the caller's key, and translate the reply back, recording the trace the
// same way a plainly proxied call does.
func (t *tap) serveWire(w http.ResponseWriter, r *http.Request, target *url.URL, wc wireCall) {
	seq := t.next()
	start := time.Now()
	t.markStart(seq, start)

	body, _ := io.ReadAll(r.Body)
	chatBody, stream, err := wc.toChat(body)
	if err != nil {
		http.Error(w, "wire translate: "+err.Error(), http.StatusBadRequest)
		t.recordLatency(seq, start, time.Now(), time.Time{}, wc.chatPath, http.StatusBadRequest, 0, false)
		return
	}
	// The translated body is forwarded and recorded as-is: the proxy is a
	// transparent tap on this path too, so a wire-translated tool reaches the
	// upstream under its own sampling, the same as every pass-through tool.
	t.writeJSON(t.reqs, reqRecord{
		Seq:    seq,
		TS:     nowStamp(),
		Method: http.MethodPost,
		Path:   wc.chatPath + " " + wc.label,
		Body:   sanitize(chatBody),
	})

	up := *target
	up.Path = singleJoin(target.Path, wc.chatPath)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, up.String(), bytes.NewReader(chatBody))
	if err != nil {
		http.Error(w, "wire upstream: "+err.Error(), http.StatusBadGateway)
		t.recordLatency(seq, start, time.Now(), time.Time{}, wc.chatPath, http.StatusBadGateway, 0, false)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	// Carry the caller's credential onward, whichever way the wire delivers it.
	// Responses sends a bearer; Anthropic sends x-api-key; Gemini sends
	// x-goog-api-key or a ?key= query param. The chat upstream wants a bearer, so
	// fold the others in when there is no Authorization header.
	switch {
	case r.Header.Get("Authorization") != "":
		req.Header.Set("Authorization", r.Header.Get("Authorization"))
	case r.Header.Get("x-api-key") != "":
		req.Header.Set("Authorization", "Bearer "+r.Header.Get("x-api-key"))
	case r.Header.Get("x-goog-api-key") != "":
		req.Header.Set("Authorization", "Bearer "+r.Header.Get("x-goog-api-key"))
	case r.URL.Query().Get("key") != "":
		req.Header.Set("Authorization", "Bearer "+r.URL.Query().Get("key"))
	}
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}

	resp, err := t.client.Do(req)
	if err != nil {
		http.Error(w, "wire upstream: "+err.Error(), http.StatusBadGateway)
		t.recordLatency(seq, start, time.Now(), time.Time{}, wc.chatPath, http.StatusBadGateway, 0, false)
		return
	}
	defer resp.Body.Close()

	// An upstream error is not ours to reshape: pass status and body through so the
	// tool sees the real failure, and still log a latency row for the attempt.
	if resp.StatusCode != http.StatusOK {
		w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
		w.WriteHeader(resp.StatusCode)
		b, _ := io.ReadAll(resp.Body)
		w.Write(b)
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		t.recordLatency(seq, start, time.Now(), time.Now(), wc.chatPath, resp.StatusCode, retryAfter, false)
		return
	}

	if stream {
		t.streamWire(w, resp, seq, start, wc)
		return
	}
	t.jsonWire(w, resp, seq, start, wc)
}

// jsonWire translates a single (non-streamed) chat completion back into the
// wire's shape and records usage and latency.
func (t *tap) jsonWire(w http.ResponseWriter, resp *http.Response, seq int, start time.Time, wc wireCall) {
	b, _ := io.ReadAll(resp.Body)
	first := time.Now()
	out, _ := json.Marshal(wc.toJSON(b, seq))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(out)
	u := extractUsage(b)
	if u != nil {
		u.Seq = seq
		u.TS = nowStamp()
		u.Status = http.StatusOK
		t.writeJSON(t.usage, u)
	}
	// A 200 reply with no usage is a dropped completion the wire had nothing to
	// reshape; flag it as an upstream fault so the harness discounts the attempt.
	t.recordLatency(seq, start, time.Now(), first, wc.chatPath, http.StatusOK, 0, u == nil)
}

// streamWire re-emits the upstream chat SSE stream in the wire's shape, recording
// usage and latency once the stream ends. The translator handles the SSE
// reshaping; the tap only supplies the flush and first-byte callbacks and turns
// the returned usage block into a trace row.
func (t *tap) streamWire(w http.ResponseWriter, resp *http.Response, seq int, start time.Time, wc wireCall) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	fl, _ := w.(http.Flusher)
	flush := func() {
		if fl != nil {
			fl.Flush()
		}
	}
	var firstAt time.Time
	onFirst := func() {
		if firstAt.IsZero() {
			firstAt = time.Now()
		}
	}
	usage := wc.toStream(w, flush, resp.Body, seq, onFirst)
	done := time.Now()

	if len(usage) > 0 {
		synth := append(append([]byte(`{"usage":`), usage...), '}')
		if u := extractUsage(synth); u != nil {
			u.Seq = seq
			u.TS = nowStamp()
			u.Status = http.StatusOK
			t.writeJSON(t.usage, u)
		}
	}
	// No usage block means the upstream stream ended before it reported one, the
	// fingerprint of a completion dropped mid-flight; flag it for the harness.
	t.recordLatency(seq, start, done, firstAt, wc.chatPath, http.StatusOK, 0, len(usage) == 0)
}
