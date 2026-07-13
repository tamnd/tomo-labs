package codex

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// Catalog is the model list Codex caches at ~/.codex/models_cache.json. The
// lab reads it to discover which gpt-5.x models the subscription can reach and
// which reasoning efforts each one takes, so a run can be pointed at, say,
// gpt-5.5 at high effort without hard-coding the roster.
type Catalog struct {
	FetchedAt     string  `json:"fetched_at"`
	ClientVersion string  `json:"client_version"`
	Models        []Model `json:"models"`
}

// Model is one entry in the catalog. Slug is the id passed to Codex; Efforts
// are the reasoning levels it accepts, with DefaultEffort the one it uses when
// none is asked for.
type Model struct {
	Slug          string   `json:"slug"`
	DisplayName   string   `json:"display_name"`
	Description   string   `json:"description"`
	DefaultEffort string   `json:"default_reasoning_level"`
	Efforts       []string `json:"-"`
	ContextWindow int      `json:"context_window"`
	MaxContext    int      `json:"max_context_window"`
	Visibility    string   `json:"visibility"`
	SupportedAPI  bool     `json:"supported_in_api"`
	Priority      int      `json:"priority"`
}

// Selectable reports whether the model is one a run should be able to pick: it
// is shown in the picker rather than hidden, so internal helpers like a review
// model do not show up as a choice.
func (m Model) Selectable() bool {
	return m.Visibility == "list"
}

// ParseCatalogFile reads and parses a models_cache.json file at path.
func ParseCatalogFile(path string) (*Catalog, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c, err := ParseCatalog(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// ParseCatalog parses the bytes of a models_cache.json file. The reasoning
// efforts sit under supported_reasoning_levels as objects, so they are pulled
// out into the flat Efforts slice the lab uses.
func ParseCatalog(b []byte) (*Catalog, error) {
	var raw struct {
		FetchedAt     string `json:"fetched_at"`
		ClientVersion string `json:"client_version"`
		Models        []struct {
			Model
			Levels []struct {
				Effort string `json:"effort"`
			} `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	c := &Catalog{FetchedAt: raw.FetchedAt, ClientVersion: raw.ClientVersion}
	for _, m := range raw.Models {
		model := m.Model
		for _, lv := range m.Levels {
			model.Efforts = append(model.Efforts, lv.Effort)
		}
		c.Models = append(c.Models, model)
	}
	return c, nil
}

// Selectable returns the models a run may pick, best first, so the discovery
// list leads with the model Codex itself defaults to. Codex ranks by a
// priority field where a lower number is a higher rank (gpt-5.6-sol is 1), so
// the sort is ascending, which also matches the order Codex lists them in its
// own picker.
func (c *Catalog) Selectable() []Model {
	var out []Model
	for _, m := range c.Models {
		if m.Selectable() {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Priority < out[j].Priority
	})
	return out
}

// Find returns the model with the given slug and whether it was present.
func (c *Catalog) Find(slug string) (Model, bool) {
	for _, m := range c.Models {
		if m.Slug == slug {
			return m, true
		}
	}
	return Model{}, false
}

// SupportsEffort reports whether the model accepts the named reasoning effort.
func (m Model) SupportsEffort(effort string) bool {
	for _, e := range m.Efforts {
		if e == effort {
			return true
		}
	}
	return false
}
