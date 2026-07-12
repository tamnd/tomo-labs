package lab

import (
	"reflect"
	"testing"
)

func TestSplitEditedTests(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		wantReason string
		wantEdited []string
	}{
		{
			name:       "no marker",
			in:         "PASS: fail_to_pass green\n",
			wantReason: "PASS: fail_to_pass green\n",
			wantEdited: nil,
		},
		{
			name:       "one marker line lifted out",
			in:         "EDITED_TESTS: tests/test_a.py tests/test_b.py\nPASS: green",
			wantReason: "PASS: green",
			wantEdited: []string{"tests/test_a.py", "tests/test_b.py"},
		},
		{
			name:       "marker below the verdict",
			in:         "FAIL: hidden tests not satisfied\nEDITED_TESTS: t/x_test.py\n",
			wantReason: "FAIL: hidden tests not satisfied\n",
			wantEdited: []string{"t/x_test.py"},
		},
		{
			name:       "marker with leading whitespace still parsed",
			in:         "  EDITED_TESTS:  spec/foo_spec.rb  \nok",
			wantReason: "ok",
			wantEdited: []string{"spec/foo_spec.rb"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reason, edited := splitEditedTests(c.in)
			if reason != c.wantReason {
				t.Errorf("reason = %q, want %q", reason, c.wantReason)
			}
			if !reflect.DeepEqual(edited, c.wantEdited) {
				t.Errorf("edited = %v, want %v", edited, c.wantEdited)
			}
		})
	}
}
