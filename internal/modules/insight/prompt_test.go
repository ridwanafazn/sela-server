package insight

import (
	"strings"
	"testing"
)

func TestInsightPromptV2ContainsFrameworkGuidance(t *testing.T) {
	var prompt strings.Builder
	if err := insightTemplate.Execute(&prompt, struct {
		Language    string
		PeriodStart string
		PeriodEnd   string
		Focus       string
		Context     string
	}{
		Language:    "en",
		PeriodStart: "2026-11-01",
		PeriodEnd:   "2026-11-30",
		Focus:       "none",
		Context:     "<journal id=\"source\">A source.</journal>",
	}); err != nil {
		t.Fatalf("render insight prompt: %v", err)
	}

	required := []string{
		"Reflection approach",
		"Life Priority Framework",
		"Do not name a framework",
		"suggested_direction",
		"Do not diagnose",
	}
	for _, want := range required {
		if !strings.Contains(prompt.String(), want) {
			t.Errorf("insight prompt is missing %q", want)
		}
	}
}
