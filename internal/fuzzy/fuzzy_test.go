package fuzzy

import "testing"

func TestMatcherEmptyQuerySortsByBaseScore(t *testing.T) {
	m := New([]Candidate{
		{ID: 0, Text: "older", BaseScore: 1.0},
		{ID: 1, Text: "newer", BaseScore: 2.8},
	})
	res := m.Match("", 0)
	if len(res) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res))
	}
	if res[0].Text != "newer" {
		t.Fatalf("expected newer first, got %s", res[0].Text)
	}
}

func TestMatcherFuzzyAndBoundaryAndUnicode(t *testing.T) {
	m := New([]Candidate{
		{ID: 0, Text: "connection-pool", BaseScore: 1.0},
		{ID: 1, Text: "my-long-unrelated-canonical-old-option-list", BaseScore: 1.0},
		{ID: 2, Text: "time&-时与", BaseScore: 1.0},
	})

	res := m.Match("cpool", 0)
	if len(res) == 0 || res[0].Text != "connection-pool" {
		t.Fatalf("expected connection-pool to rank first, got %+v", res)
	}

	ures := m.Match("时与", 0)
	if len(ures) != 1 || ures[0].Text != "time&-时与" {
		t.Fatalf("expected unicode match for time&-时与, got %+v", ures)
	}
	if len(ures[0].HighlightPositions) != 2 || ures[0].HighlightPositions[0] != 6 || ures[0].HighlightPositions[1] != 7 {
		t.Fatalf("unexpected rune highlight positions: %+v", ures[0].HighlightPositions)
	}
}
