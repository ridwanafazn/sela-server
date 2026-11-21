package journalanalysis

import (
	"strings"
	"testing"
)

func TestJournalAnalysisPromptContainsRules(t *testing.T) {
	var prompt strings.Builder
	if err := analysisTemplate.Execute(&prompt, struct {
		Language  string
		EntryDate string
		Title     string
		Content   string
	}{
		Language:  "en",
		EntryDate: "2026-11-21",
		Title:     "A difficult day",
		Content:   "A journal entry.",
	}); err != nil {
		t.Fatalf("render journal analysis prompt: %v", err)
	}

	required := []string{
		"single journal entry",
		"Do not diagnose",
		"observation",
		"interpretation",
		"evidence_note",
		"Life Priority Framework",
		"Do not name a framework",
	}
	for _, want := range required {
		if !strings.Contains(prompt.String(), want) {
			t.Errorf("journal analysis prompt is missing %q", want)
		}
	}
}
