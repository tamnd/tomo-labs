package main

import (
	"strings"
	"testing"

	"github.com/tamnd/tomo-labs/pkg/analyzer/codex"
)

func TestModelList(t *testing.T) {
	got := modelList([]codex.ModelUse{
		{Model: "gpt-5.4-mini", Effort: "medium"},
		{Model: "gpt-5.6-luna", Effort: ""},
	})
	if got != "gpt-5.4-mini/medium, gpt-5.6-luna" {
		t.Errorf("modelList = %q", got)
	}
	if got := modelList(nil); got != "(none)" {
		t.Errorf("empty modelList = %q, want (none)", got)
	}
}

func TestByToolLine(t *testing.T) {
	// Most-used first, ties broken by name, so the shape reads at a glance.
	got := byToolLine(map[string]int{"apply_patch": 1, "exec_command": 29, "read": 29})
	if got != "exec_command=29 read=29 apply_patch=1" {
		t.Errorf("byToolLine = %q", got)
	}
}

func TestOutcome(t *testing.T) {
	if o := outcome(codex.Summary{Complete: true}); o != "complete" {
		t.Errorf("complete = %q", o)
	}
	if o := outcome(codex.Summary{Aborted: true}); o != "aborted" {
		t.Errorf("aborted = %q", o)
	}
	if o := outcome(codex.Summary{}); o != "unknown" {
		t.Errorf("neither = %q, want unknown", o)
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("  hello\nworld", 100); got != "hello" {
		t.Errorf("firstLine = %q, want hello", got)
	}
	long := strings.Repeat("x", 200)
	got := firstLine(long, 10)
	if len(got) != 13 || !strings.HasSuffix(got, "...") {
		t.Errorf("firstLine cap = %q (len %d), want 10 chars + ...", got, len(got))
	}
}
