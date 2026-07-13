package codex

import (
	"os"
	"path/filepath"
	"testing"
)

// sampleCatalog mirrors the real models_cache.json shape: a listed frontier
// model with four efforts, a listed older model, and a hidden internal helper
// that must not show up as a choice.
const sampleCatalog = `{
  "fetched_at": "2026-07-07T15:50:00Z",
  "client_version": "0.142.3",
  "models": [
    {
      "slug": "gpt-5.5",
      "display_name": "GPT-5.5",
      "default_reasoning_level": "medium",
      "supported_reasoning_levels": [
        {"effort": "low"}, {"effort": "medium"}, {"effort": "high"}, {"effort": "xhigh"}
      ],
      "context_window": 258400,
      "max_context_window": 400000,
      "visibility": "list",
      "supported_in_api": true,
      "priority": 7
    },
    {
      "slug": "gpt-5.4",
      "display_name": "GPT-5.4",
      "default_reasoning_level": "medium",
      "supported_reasoning_levels": [{"effort": "medium"}, {"effort": "high"}],
      "context_window": 258400,
      "visibility": "list",
      "priority": 5
    },
    {
      "slug": "codex-auto-review",
      "display_name": "Auto Review",
      "visibility": "hidden",
      "priority": 1
    }
  ]
}`

func parseSampleCatalog(t *testing.T) *Catalog {
	t.Helper()
	c, err := ParseCatalog([]byte(sampleCatalog))
	if err != nil {
		t.Fatalf("ParseCatalog: %v", err)
	}
	return c
}

func TestParseCatalog(t *testing.T) {
	c := parseSampleCatalog(t)
	if c.ClientVersion != "0.142.3" {
		t.Errorf("client version = %q, want 0.142.3", c.ClientVersion)
	}
	if len(c.Models) != 3 {
		t.Fatalf("models = %d, want 3", len(c.Models))
	}
	m, ok := c.Find("gpt-5.5")
	if !ok {
		t.Fatal("gpt-5.5 not found")
	}
	if m.DefaultEffort != "medium" {
		t.Errorf("default effort = %q, want medium", m.DefaultEffort)
	}
	if !m.SupportsEffort("xhigh") || m.SupportsEffort("insane") {
		t.Errorf("efforts = %v, want to accept xhigh and reject insane", m.Efforts)
	}
	if m.MaxContext != 400000 {
		t.Errorf("max context = %d, want 400000", m.MaxContext)
	}
}

func TestCatalogSelectableHidesInternalAndSortsByPriority(t *testing.T) {
	c := parseSampleCatalog(t)
	sel := c.Selectable()
	if len(sel) != 2 {
		t.Fatalf("selectable = %d, want 2 (hidden helper excluded)", len(sel))
	}
	if sel[0].Slug != "gpt-5.5" || sel[1].Slug != "gpt-5.4" {
		t.Errorf("selectable order = %s,%s, want gpt-5.5 then gpt-5.4 by priority", sel[0].Slug, sel[1].Slug)
	}
}

func TestParseCatalogFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "models_cache.json")
	if err := os.WriteFile(path, []byte(sampleCatalog), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := ParseCatalogFile(path)
	if err != nil {
		t.Fatalf("ParseCatalogFile: %v", err)
	}
	if len(c.Models) != 3 {
		t.Errorf("models = %d, want 3", len(c.Models))
	}
}
