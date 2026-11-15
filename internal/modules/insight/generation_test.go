package insight

import (
	"errors"
	"strings"
	"testing"

	"github.com/ridwanafazn/sela-server/internal/modules/ai"
)

func TestValidateInsightOutput(t *testing.T) {
	journalID := "11111111-1111-1111-1111-111111111111"
	messageID := "22222222-2222-2222-2222-222222222222"
	sources := []SourceRecord{
		{Type: "journal", ID: journalID, Content: "I could not sleep well because of work."},
		{Type: "message", ID: messageID, Content: "I want to protect my evenings."},
	}
	included := map[string]bool{
		sourceKey("journal", journalID): true,
		sourceKey("message", messageID): true,
	}

	valid := &insightOutput{
		Title:              "Evenings under pressure",
		Summary:            "Work is affecting sleep and evenings.",
		Observation:        "Work appears in both records.",
		Interpretation:     "Evening boundaries may be difficult right now.",
		Confidence:         0.62,
		ReflectionQuestion: "What would one protected evening look like?",
		SuggestedDirection: DirectionImprove,
		Evidence: []outputEvidence{
			{SourceType: "journal", SourceID: journalID, Note: "The journal links work to poor sleep."},
			{SourceType: "message", SourceID: messageID, Note: "The message states a wish to protect evenings."},
		},
	}

	validated, reason, err := validateInsightOutput(valid, sources, included)
	if err != nil {
		t.Fatalf("valid output: %v", err)
	}
	if reason != "" {
		t.Errorf("valid output reason = %q", reason)
	}
	if validated == nil || len(validated.Evidence) != 2 {
		t.Errorf("validated = %+v", validated)
	}

	oneEvidence := *valid
	oneEvidence.Evidence = oneEvidence.Evidence[:1]
	if _, reason, err := validateInsightOutput(&oneEvidence, sources, included); err != nil || reason == "" {
		t.Errorf("one evidence = reason %q, err %v; want insufficient", reason, err)
	}

	lowConfidence := *valid
	lowConfidence.Confidence = 0.39
	if _, reason, err := validateInsightOutput(&lowConfidence, sources, included); err != nil || reason == "" {
		t.Errorf("low confidence = reason %q, err %v; want insufficient", reason, err)
	}

	invented := *valid
	invented.Evidence = append([]outputEvidence(nil), valid.Evidence...)
	invented.Evidence[0].SourceID = "33333333-3333-3333-3333-333333333333"
	if _, _, err := validateInsightOutput(&invented, sources, included); !errors.Is(err, ai.ErrInvalidResponse) {
		t.Errorf("invented source error = %v, want ErrInvalidResponse", err)
	}

	duplicate := *valid
	duplicate.Evidence = []outputEvidence{valid.Evidence[0], valid.Evidence[0]}
	if _, reason, err := validateInsightOutput(&duplicate, sources, included); err != nil || reason == "" {
		t.Errorf("duplicate evidence = reason %q, err %v; want insufficient", reason, err)
	}

	longNote := *valid
	longNote.Evidence = append([]outputEvidence(nil), valid.Evidence...)
	longNote.Evidence[0].Note = strings.Repeat("a", 301)
	if _, _, err := validateInsightOutput(&longNote, sources, included); !errors.Is(err, ai.ErrInvalidResponse) {
		t.Errorf("long note error = %v, want ErrInvalidResponse", err)
	}

	emptyTitle := *valid
	emptyTitle.Title = " "
	if _, _, err := validateInsightOutput(&emptyTitle, sources, included); !errors.Is(err, ai.ErrInvalidResponse) {
		t.Errorf("empty title error = %v, want ErrInvalidResponse", err)
	}
}

func TestParseInsightResponse(t *testing.T) {
	valid := `{"title":"T","summary":"S","observation":"O","interpretation":"I","confidence":0.7,"reflection_question":"Q?","suggested_direction":"improve","evidence":[]}`
	output, err := parseInsightResponse("```json\n" + valid + "\n```")
	if err != nil {
		t.Fatalf("parse fenced insight: %v", err)
	}
	if output.Title != "T" {
		t.Errorf("title = %q", output.Title)
	}
	if _, err := parseInsightResponse("not json"); !errors.Is(err, ai.ErrInvalidResponse) {
		t.Errorf("invalid JSON error = %v, want ErrInvalidResponse", err)
	}
}

func TestBuildInsightContextBoundsSources(t *testing.T) {
	sources := []SourceRecord{
		{Type: "journal", ID: "11111111-1111-1111-1111-111111111111", Content: strings.Repeat("long content ", 5000)},
		{Type: "message", ID: "22222222-2222-2222-2222-222222222222", Content: "A short message."},
	}
	contextText, included := buildInsightContext(sources)
	if len(included) < 2 {
		t.Errorf("included = %d, want at least 2", len(included))
	}
	if len([]rune(contextText)) > contextRuneBudget {
		t.Errorf("context length = %d, want <= %d", len([]rune(contextText)), contextRuneBudget)
	}
}
