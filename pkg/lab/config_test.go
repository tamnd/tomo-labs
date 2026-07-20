package lab

import "testing"

func TestPrepTimeoutConfig(t *testing.T) {
	t.Setenv("LAB_PREP_TIMEOUT", "")
	if got := DefaultConfig().PrepSecs; got != 300 {
		t.Fatalf("default prep timeout = %d, want 300", got)
	}

	t.Setenv("LAB_PREP_TIMEOUT", "45")
	if got := DefaultConfig().PrepSecs; got != 45 {
		t.Fatalf("configured prep timeout = %d, want 45", got)
	}

	t.Setenv("LAB_PREP_TIMEOUT", "0")
	if got := DefaultConfig().PrepSecs; got != 0 {
		t.Fatalf("disabled prep timeout = %d, want 0", got)
	}
}
