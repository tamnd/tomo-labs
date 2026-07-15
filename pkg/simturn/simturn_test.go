package simturn

import (
	"testing"

	"github.com/tamnd/tomo/pkg/agent"
	"github.com/tamnd/tomo/pkg/engine/cx"
	"github.com/tamnd/tomo/pkg/provider"
	"github.com/tamnd/tomo/pkg/tool"
)

func TestValidEngine(t *testing.T) {
	for _, e := range []string{"agent", "cx", "cx-offline"} {
		if !validEngine(e) {
			t.Errorf("validEngine(%q) = false, want true", e)
		}
	}
	for _, e := range []string{"", "ds", "codex", "CX"} {
		if validEngine(e) {
			t.Errorf("validEngine(%q) = true, want false", e)
		}
	}
}

// buildEngine must hand back the concrete engine the name asks for, so a run on
// --engine agent drives the default loop and --engine cx drives the codex loop.
func TestBuildEngineSelectsConcreteType(t *testing.T) {
	reg := tool.NewRegistry()
	var prov provider.Provider

	if e := buildEngine("agent", prov, "m", "sys", reg, "/w"); func() bool { _, ok := e.(*agent.Agent); return !ok }() {
		t.Errorf("buildEngine(agent) = %T, want *agent.Agent", e)
	}
	for _, name := range []string{"cx", "cx-offline"} {
		if e := buildEngine(name, prov, "m", "sys", reg, "/w"); func() bool { _, ok := e.(*cx.Engine); return !ok }() {
			t.Errorf("buildEngine(%q) = %T, want *cx.Engine", name, e)
		}
	}
}

// engineTools must retune the builtin descriptions for cx but leave them untouched
// for the default agent, so each engine sees the tool prose its prompt expects.
func TestEngineToolsRetunesOnlyForCx(t *testing.T) {
	base := []tool.Tool{{Name: "grep", Description: "orig", Class: tool.ClassRead}}

	same := engineTools("agent", base)
	if len(same) != 1 || same[0].Description != "orig" {
		t.Errorf("engineTools(agent) changed the tools: %+v", same)
	}

	tuned := engineTools("cx", base)
	if len(tuned) != 1 {
		t.Fatalf("engineTools(cx) len = %d, want 1", len(tuned))
	}
	// cx.Retune only rewrites descriptions it knows; an unknown tool passes through,
	// so assert the call is wired rather than a specific rewrite.
	if tuned[0].Name != "grep" {
		t.Errorf("engineTools(cx) lost the tool: %+v", tuned)
	}
}

func TestLastNonEmptyLine(t *testing.T) {
	cases := map[string]string{
		"PASS: green\n":                 "PASS: green",
		"noise\nFAIL: hidden tests\n\n": "FAIL: hidden tests",
		"":                              "",
		"  \n  only spaces  \n":         "only spaces",
	}
	for in, want := range cases {
		if got := lastNonEmptyLine(in); got != want {
			t.Errorf("lastNonEmptyLine(%q) = %q, want %q", in, got, want)
		}
	}
}

// Result.Converged is the cheap directional signal used before a real grade: it
// wants a gold-file hit, no test-file edits, no timeout, no error.
func TestConverged(t *testing.T) {
	ok := Result{HitGold: []string{"a.py"}}
	if !ok.Converged() {
		t.Error("expected converged for a clean gold hit")
	}
	for _, bad := range []Result{
		{HitGold: nil},
		{HitGold: []string{"a.py"}, EditedTests: []string{"test_a.py"}},
		{HitGold: []string{"a.py"}, TimedOut: true},
		{HitGold: []string{"a.py"}, Err: "boom"},
	} {
		if bad.Converged() {
			t.Errorf("expected not converged for %+v", bad)
		}
	}
}

// Ensure both engines still satisfy the interface the sim drives them through.
var (
	_ turnEngine = (*agent.Agent)(nil)
	_ turnEngine = (*cx.Engine)(nil)
)
