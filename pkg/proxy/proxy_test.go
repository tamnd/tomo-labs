package proxy

import (
	"net/http"
	"testing"
	"time"
)

// A plain OpenAI usage block yields the token counts, the nested cached-prompt
// count, and the reported cost.
func TestExtractUsageOpenAI(t *testing.T) {
	body := []byte(`{"usage":{"prompt_tokens":100,"completion_tokens":40,"total_tokens":140,"prompt_tokens_details":{"cached_tokens":64},"completion_tokens_details":{"reasoning_tokens":24},"cost":0.0123}}`)
	u := extractUsage(body)
	if u == nil {
		t.Fatal("no usage extracted")
	}
	if u.PromptTokens != 100 || u.CompletionTokens != 40 || u.TotalTokens != 140 {
		t.Errorf("tokens = %d/%d/%d, want 100/40/140", u.PromptTokens, u.CompletionTokens, u.TotalTokens)
	}
	if u.CachedTokens != 64 {
		t.Errorf("cached = %d, want 64", u.CachedTokens)
	}
	if u.ReasoningTokens != 24 {
		t.Errorf("reasoning = %d, want 24", u.ReasoningTokens)
	}
	if u.CostUSD != 0.0123 {
		t.Errorf("cost = %v, want 0.0123", u.CostUSD)
	}
}

func TestExtractUsageResponsesReasoning(t *testing.T) {
	body := []byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":90,\"output_tokens\":30,\"total_tokens\":120,\"input_tokens_details\":{\"cached_tokens\":50},\"output_tokens_details\":{\"reasoning_tokens\":20}}}}\n\n")
	u := extractUsage(body)
	if u == nil || u.ReasoningTokens != 20 || u.CachedTokens != 50 || u.TotalTokens != 120 {
		t.Fatalf("usage = %+v, want reasoning/cache/total 20/50/120", u)
	}
}

// The Anthropic shape names cache read and write tokens flat, and extractUsage
// maps them onto the same fields.
func TestExtractUsageAnthropicCache(t *testing.T) {
	body := []byte(`{"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"cache_read_input_tokens":80,"cache_creation_input_tokens":120}}`)
	u := extractUsage(body)
	if u == nil {
		t.Fatal("no usage extracted")
	}
	if u.CachedTokens != 80 {
		t.Errorf("cached = %d, want 80", u.CachedTokens)
	}
	if u.CacheWriteTokens != 120 {
		t.Errorf("cache_write = %d, want 120", u.CacheWriteTokens)
	}
}

// A streamed reply carries usage on the last data: line, and a provider that
// reports no cache or cost leaves those fields zero rather than guessing.
func TestExtractUsageStreamNoCache(t *testing.T) {
	body := []byte("data: {\"choices\":[{}]}\n\ndata: {\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":3,\"total_tokens\":10}}\n\ndata: [DONE]\n")
	u := extractUsage(body)
	if u == nil {
		t.Fatal("no usage extracted")
	}
	if u.TotalTokens != 10 {
		t.Errorf("total = %d, want 10", u.TotalTokens)
	}
	if u.CachedTokens != 0 || u.CacheWriteTokens != 0 || u.CostUSD != 0 {
		t.Errorf("unreported cache/cost should stay zero, got %d/%d/%v", u.CachedTokens, u.CacheWriteTokens, u.CostUSD)
	}
}

// A completion the upstream dropped mid-stream is caught two ways: an error
// object sent as an SSE data line, and a stream that stops before its [DONE] and
// usage. A clean stream (usage plus [DONE]) and a complete non-streamed object
// are never flagged, so a healthy reply is not mistaken for a fault.
func TestStreamFailed(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		hasUsage bool
		want     bool
		wantMsg  string
	}{
		{
			name:    "error payload as data line",
			body:    "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: {\"error\":{\"message\":\"Streaming response failed\",\"type\":\"server_error\"}}\n\n",
			want:    true,
			wantMsg: "Streaming response failed",
		},
		{
			name: "truncated stream, no done and no usage",
			body: "data: {\"choices\":[{\"delta\":{\"content\":\"par\"}}]}\n\n",
			want: true, wantMsg: "truncated stream",
		},
		{
			name:     "clean stream with usage and done",
			body:     "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: {\"usage\":{\"total_tokens\":9}}\n\ndata: [DONE]\n",
			hasUsage: true,
			want:     false,
		},
		{
			name:     "non-streamed object reply",
			body:     `{"choices":[{"message":{"content":"hi"}}],"usage":{"total_tokens":9}}`,
			hasUsage: true,
			want:     false,
		},
	}
	for _, c := range cases {
		bad, msg := streamFailed([]byte(c.body), c.hasUsage)
		if bad != c.want {
			t.Errorf("%s: streamFailed = %v, want %v", c.name, bad, c.want)
		}
		if c.wantMsg != "" && msg != c.wantMsg {
			t.Errorf("%s: msg = %q, want %q", c.name, msg, c.wantMsg)
		}
	}
}

// zen sends Retry-After as plain delay-seconds on a 429, so that form parses
// straight through; an HTTP-date form some other upstream might send still
// resolves to a positive second count, and anything unparseable or absent
// reports 0 rather than guessing.
func TestParseRetryAfter(t *testing.T) {
	if n := parseRetryAfter("17600"); n != 17600 {
		t.Errorf("delay-seconds: got %d, want 17600", n)
	}
	if n := parseRetryAfter(""); n != 0 {
		t.Errorf("empty header: got %d, want 0", n)
	}
	if n := parseRetryAfter("not a number"); n != 0 {
		t.Errorf("garbage header: got %d, want 0", n)
	}
	future := time.Now().Add(2 * time.Hour).UTC().Format(http.TimeFormat)
	if n := parseRetryAfter(future); n < 7100 || n > 7300 {
		t.Errorf("HTTP-date two hours out: got %d, want ~7200", n)
	}
}
