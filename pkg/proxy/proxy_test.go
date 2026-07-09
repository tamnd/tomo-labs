package proxy

import (
	"encoding/json"
	"testing"
)

// A completion request gets the sampling knobs forced, whatever it sent, and
// every other field is left as it was.
func TestDeterminismForcesSamplingOnCompletion(t *testing.T) {
	d := &determinism{fields: map[string]json.RawMessage{
		"temperature": json.RawMessage("0"),
		"top_p":       json.RawMessage("1"),
		"seed":        json.RawMessage("7"),
	}}
	in := []byte(`{"model":"m","temperature":0.9,"messages":[{"role":"user","content":"hi"}]}`)
	out := d.apply(in)

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("output not JSON: %v", err)
	}
	if got["temperature"] != float64(0) {
		t.Errorf("temperature = %v, want 0", got["temperature"])
	}
	if got["top_p"] != float64(1) {
		t.Errorf("top_p = %v, want 1", got["top_p"])
	}
	if got["seed"] != float64(7) {
		t.Errorf("seed = %v, want 7", got["seed"])
	}
	if got["model"] != "m" {
		t.Errorf("model = %v, want it left intact", got["model"])
	}
	if _, ok := got["messages"]; !ok {
		t.Error("messages field was dropped")
	}
}

// Anything that is not a completion request is forwarded byte-for-byte: a plain
// JSON object with no messages or prompt, and a non-JSON body.
func TestDeterminismLeavesNonCompletionAlone(t *testing.T) {
	d := &determinism{fields: map[string]json.RawMessage{"temperature": json.RawMessage("0")}}
	for _, body := range []string{`{"hello":"world"}`, `not json at all`, ``} {
		if got := string(d.apply([]byte(body))); got != body {
			t.Errorf("apply(%q) = %q, want it unchanged", body, got)
		}
	}
}

// A nil determinism (feature off) is a no-op.
func TestDeterminismOffIsPassthrough(t *testing.T) {
	var d *determinism
	body := []byte(`{"messages":[],"temperature":0.9}`)
	if got := string(d.apply(body)); got != string(body) {
		t.Errorf("apply with det off = %q, want unchanged", got)
	}
}
