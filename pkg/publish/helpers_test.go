package publish

import (
	"encoding/json"
	"os"
	"testing"
)

// writeFile writes a file for a test, failing the test on error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// mustJSON unmarshals a JSON line into v, failing the test on error.
func mustJSON(t *testing.T, line string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(line), v); err != nil {
		t.Fatalf("unmarshal %q: %v", line, err)
	}
}
