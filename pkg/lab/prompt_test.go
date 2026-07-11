package lab

import "testing"

// TestCollectPromptsRanksAgentFirst covers the common shape: a tool makes a small
// side call (opencode's title generator) and many agent calls carrying a tool
// schema. The agent prompt must rank first, count its requests, and list its
// tools; the side prompt follows.
func TestCollectPromptsRanksAgentFirst(t *testing.T) {
	agent := `{"method":"POST","path":"/zen/v1/chat/completions","body":{"messages":[` +
		`{"role":"system","content":"You are the coding agent."},` +
		`{"role":"user","content":"do it"}],"tools":[{"function":{"name":"read"}},{"function":{"name":"write"}}]}}`
	side := `{"method":"POST","path":"/zen/v1/chat/completions","body":{"messages":[` +
		`{"role":"system","content":"You are a title generator."},` +
		`{"role":"user","content":"name this"}]}}`
	f := writeReqs(t, agent, agent, side)

	got := collectPrompts([]string{f})
	if len(got) != 2 {
		t.Fatalf("want 2 distinct prompts, got %d", len(got))
	}
	if !got[0].WithTools || got[0].Text != "You are the coding agent." {
		t.Errorf("agent prompt should rank first, got %+v", got[0])
	}
	if got[0].Requests != 2 {
		t.Errorf("agent prompt request count: want 2, got %d", got[0].Requests)
	}
	if len(got[0].Tools) != 2 || got[0].Tools[0] != "read" || got[0].Tools[1] != "write" {
		t.Errorf("agent tools: want [read write] sorted, got %v", got[0].Tools)
	}
	if got[1].WithTools || got[1].Text != "You are a title generator." {
		t.Errorf("side prompt should rank last, got %+v", got[1])
	}
}

// TestCollectPromptsGroupsRenderings collapses renderings of one base prompt that
// differ only in volatile spans (the date, the working dir, a session id) into a
// single prompt, counting the renderings as Variants while keeping one fully
// rendered representative.
func TestCollectPromptsGroupsRenderings(t *testing.T) {
	tmpl := func(date, dir, id string) string {
		body := `You are the agent. Today is ` + date + `. cwd is ` + dir + `. session ` + id + `.`
		return `{"method":"POST","path":"/v1/chat/completions","body":{"messages":[` +
			`{"role":"system","content":"` + body + `"},{"role":"user","content":"go"}],` +
			`"tools":[{"function":{"name":"read"}}]}}`
	}
	r1 := tmpl("2026-07-10", "/home/work/a", "a1b2c3d4e5f6")
	r2 := tmpl("2026-07-11", "/home/work/bb", "0f9e8d7c6b5a")
	r3 := tmpl("2026-07-11", "/tmp/run/ccc", "deadbeef1234")
	got := collectPrompts([]string{writeReqs(t, r1, r2, r3)})
	if len(got) != 1 {
		t.Fatalf("renderings of one base prompt should group to 1, got %d", len(got))
	}
	if got[0].Variants != 3 {
		t.Errorf("Variants: want 3 renderings, got %d", got[0].Variants)
	}
	if got[0].Requests != 3 {
		t.Errorf("Requests: want 3, got %d", got[0].Requests)
	}
}

// TestPromptSig masks volatile spans so two renderings share a signature, while
// prompts with genuinely different stable text keep different signatures.
func TestPromptSig(t *testing.T) {
	a := promptSig("Today is 2026-07-10T03:21:46Z at /Users/x/proj id 9fceab12ff.")
	b := promptSig("Today is 2026-07-11T09:00:00Z at /tmp/proj/y id 001188aa55.")
	if a != b {
		t.Errorf("renderings should share a signature:\n a=%q\n b=%q", a, b)
	}
	if promptSig("You are agent A.") == promptSig("You are agent B.") {
		t.Error("different stable text must not share a signature")
	}
}

// TestParseWire reads the origin wire off the proxy's path tag, and treats an
// untagged chat request as native chat.
func TestParseWire(t *testing.T) {
	cases := map[string]string{
		"/v1/chat/completions (from responses)": "responses",
		"/v1/chat/completions (from messages)":  "messages",
		"/v1/chat/completions (from gemini)":    "gemini",
		"/zen/v1/chat/completions":              "chat",
	}
	for path, want := range cases {
		if got := parseWire(path); got != want {
			t.Errorf("parseWire(%q): want %q, got %q", path, want, got)
		}
	}
}

// TestContentTextArrayParts flattens the array-of-parts content shape some wires
// use, and a codex-style request that carries two distinct system messages must
// surface both.
func TestContentTextArrayParts(t *testing.T) {
	req := `{"method":"POST","path":"/v1/chat/completions (from responses)","body":{"messages":[` +
		`{"role":"system","content":[{"type":"text","text":"Base instructions."}]},` +
		`{"role":"developer","content":"Environment context."},` +
		`{"role":"user","content":"go"}],"tools":[{"function":{"name":"exec"}}]}}`
	got := collectPrompts([]string{writeReqs(t, req)})
	if len(got) != 2 {
		t.Fatalf("want both system messages, got %d", len(got))
	}
	texts := map[string]bool{got[0].Text: true, got[1].Text: true}
	if !texts["Base instructions."] || !texts["Environment context."] {
		t.Errorf("both system texts should surface, got %v", texts)
	}
	if got[0].Wire != "responses" {
		t.Errorf("wire should carry through, got %q", got[0].Wire)
	}
}
