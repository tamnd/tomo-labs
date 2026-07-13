package lab

import (
	"strings"
	"testing"
)

// transcribe should walk the fullest request in a tap into an ordered step list:
// system, task, the agent's reasoning, each tool call, and each result, dropping
// the shorter earlier requests in favor of the one that carries the whole history.
// It stays in this package because it reads the raw request tap the lab package
// owns; the analysis of the steps it produces is tested in the inspect package.
func TestTranscribeWalksFullestRequest(t *testing.T) {
	short := `{"seq":1,"body":{"messages":[{"role":"system","content":"you are an agent"},{"role":"user","content":"fix the bug"}]}}`
	full := `{"seq":2,"body":{"messages":[` +
		`{"role":"system","content":"you are an agent"},` +
		`{"role":"user","content":"fix the bug"},` +
		`{"role":"assistant","content":"let me look","tool_calls":[{"function":{"name":"grep","arguments":"{\"pattern\":\"foo\"}"}}]},` +
		`{"role":"tool","content":"foo.py:1: def foo()"},` +
		`{"role":"assistant","content":"","tool_calls":[{"function":{"name":"edit","arguments":"{\"path\":\"foo.py\"}"}}]},` +
		`{"role":"tool","content":"edited foo.py"}` +
		`]}}`
	p := writeReqs(t, short, full)

	steps, err := transcribe(p)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, s := range steps {
		kinds = append(kinds, s.Kind)
	}
	want := []string{"system", "user", "assistant", "call", "result", "call", "result"}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
	if steps[2].Text != "let me look" {
		t.Errorf("assistant text = %q", steps[2].Text)
	}
	if steps[3].Name != "grep" || !strings.Contains(steps[3].Text, "foo") {
		t.Errorf("first call = %+v", steps[3])
	}
	if steps[5].Name != "edit" {
		t.Errorf("second call name = %q, want edit", steps[5].Name)
	}
}
