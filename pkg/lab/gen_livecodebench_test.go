package lab

import "testing"

func TestLcbWantDifficulty(t *testing.T) {
	// No request means take any difficulty, signalled by a nil set.
	if got, err := lcbWantDifficulty(nil); err != nil || got != nil {
		t.Fatalf("empty: got %v, %v; want nil, nil", got, err)
	}

	// A request is lowercased and trimmed so it matches the dataset's casing.
	got, err := lcbWantDifficulty([]string{"Easy", " HARD "})
	if err != nil {
		t.Fatalf("mixed case: %v", err)
	}
	if !got["easy"] || !got["hard"] || got["medium"] {
		t.Fatalf("mixed case set = %v, want easy+hard only", got)
	}

	// An unknown tier is an error, not a silent set that renders zero tasks.
	if _, err := lcbWantDifficulty([]string{"ez"}); err == nil {
		t.Fatal("expected error for unknown difficulty")
	}
}

func TestLcbRowDifficultyFilter(t *testing.T) {
	rows := []lcbRow{
		{QuestionID: "a", Difficulty: "easy"},
		{QuestionID: "b", Difficulty: "Medium"},
		{QuestionID: "c", Difficulty: "hard"},
	}
	want, err := lcbWantDifficulty([]string{"medium"})
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, r := range rows {
		if want != nil && !want[normDifficulty(r.Difficulty)] {
			continue
		}
		kept = append(kept, r.QuestionID)
	}
	if len(kept) != 1 || kept[0] != "b" {
		t.Fatalf("kept %v, want [b]", kept)
	}
}
