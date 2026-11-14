package fuzzy

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

type Candidate struct {
	ID        int
	Text      string
	BaseScore float64
}

type Result struct {
	ID                 int
	Text               string
	Score              float64
	HighlightPositions []int
}

type preparedEntry struct {
	id        int
	text      string
	runes     []rune
	lower     []rune
	baseScore float64
}

type Matcher struct {
	entries   []preparedEntry
	sqrtTable [65]float64
}

func New(candidates []Candidate) *Matcher {
	m := &Matcher{
		entries: make([]preparedEntry, len(candidates)),
	}
	for i := 0; i <= 64; i++ {
		m.sqrtTable[i] = 2.0 / math.Sqrt(float64(i+1))
	}
	for i, c := range candidates {
		runes := []rune(c.Text)
		lower := []rune(strings.ToLower(c.Text))
		// Guard against rare case where ToLower changes rune length: align per rune if needed.
		if len(lower) != len(runes) {
			lower = make([]rune, len(runes))
			for idx, r := range runes {
				lower[idx] = unicode.ToLower(r)
			}
		}
		m.entries[i] = preparedEntry{
			id:        c.ID,
			text:      c.Text,
			runes:     runes,
			lower:     lower,
			baseScore: c.BaseScore,
		}
	}
	return m
}

func (m *Matcher) Match(query string, limit int) []Result {
	qLower := make([]rune, 0, len(query))
	for _, r := range query {
		qLower = append(qLower, unicode.ToLower(r))
	}

	results := make([]Result, 0, len(m.entries))
	for _, e := range m.entries {
		score, positions, ok := m.scoreEntry(e, qLower)
		if !ok {
			continue
		}
		results = append(results, Result{
			ID:                 e.id,
			Text:               e.text,
			Score:              score,
			HighlightPositions: positions,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if math.Abs(results[i].Score-results[j].Score) > 1e-9 {
			return results[i].Score > results[j].Score
		}
		li, lj := len([]rune(results[i].Text)), len([]rune(results[j].Text))
		if li != lj {
			return li < lj
		}
		return results[i].Text < results[j].Text
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

func (m *Matcher) scoreEntry(e preparedEntry, qLower []rune) (float64, []int, bool) {
	score := e.baseScore
	if len(qLower) == 0 {
		return score, nil, true
	}

	text := e.lower
	if len(qLower) > len(text) {
		return 0, nil, false
	}

	positions := make([]int, 0, len(qLower))
	lastPos := -1
	pos := 0

	for _, qc := range qLower {
		found := -1
		for idx := pos; idx < len(text); idx++ {
			if text[idx] == qc {
				found = idx
				break
			}
		}
		if found < 0 {
			return 0, nil, false
		}

		positions = append(positions, found)
		score += 1.0

		if found == 0 || !isAlphaNum(text[found-1]) {
			score += 1.0
		}

		if lastPos >= 0 {
			gap := found - lastPos - 1
			if gap >= 0 && gap <= 64 {
				score += m.sqrtTable[gap]
			} else if gap > 64 {
				score += 2.0 / math.Sqrt(float64(gap+1))
			}
		}

		lastPos = found
		pos = found + 1
	}

	if lastPos >= 0 {
		score *= float64(len(qLower)) / float64(lastPos+1)
	}
	score *= 10.0 / (float64(len(e.runes)) + 10.0)

	return score, positions, true
}

func isAlphaNum(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
