package journalanalysis

import (
	"errors"
	"strings"
	"testing"

	"github.com/ridwanafazn/sela-server/internal/modules/ai"
)

func validAnalysisOutput() *analysisOutput {
	return &analysisOutput{
		Summary:            "Work pressure and rest appear in the same entry.",
		Observation:        "The entry connects a difficult workday with poor sleep.",
		Interpretation:     "Workload may be affecting recovery, but one entry cannot prove a pattern.",
		Confidence:         0.62,
		ReflectionQuestion: "What would make tonight's wind-down easier?",
		SuggestedDirection: DirectionImprove,
		EvidenceNote:       "The entry links the workday to poor sleep.",
	}
}

func TestValidateAnalysisOutput(t *testing.T) {
	content := "I had a difficult day at work and stayed late again. I could not sleep because I kept replaying the conversation with my manager. I want to protect my evenings better."

	validated, reason, err := validateAnalysisOutput(validAnalysisOutput(), content)
	if err != nil {
		t.Fatalf("valid output: %v", err)
	}
	if reason != "" {
		t.Errorf("valid output reason = %q", reason)
	}
	if validated == nil || validated.EvidenceNote == "" {
		t.Errorf("validated = %+v", validated)
	}

	lowConfidence := validAnalysisOutput()
	lowConfidence.Confidence = 0.39
	if _, reason, err := validateAnalysisOutput(lowConfidence, content); err != nil || reason == "" {
		t.Errorf("low confidence = reason %q, err %v; want insufficient", reason, err)
	}

	copied := validAnalysisOutput()
	copied.EvidenceNote = content
	if _, _, err := validateAnalysisOutput(copied, content); !errors.Is(err, ai.ErrInvalidResponse) {
		t.Errorf("copied evidence error = %v, want ErrInvalidResponse", err)
	}

	emptySummary := validAnalysisOutput()
	emptySummary.Summary = " "
	if _, _, err := validateAnalysisOutput(emptySummary, content); !errors.Is(err, ai.ErrInvalidResponse) {
		t.Errorf("empty summary error = %v, want ErrInvalidResponse", err)
	}

	longNote := validAnalysisOutput()
	longNote.EvidenceNote = strings.Repeat("a", 301)
	if _, _, err := validateAnalysisOutput(longNote, content); !errors.Is(err, ai.ErrInvalidResponse) {
		t.Errorf("long evidence error = %v, want ErrInvalidResponse", err)
	}

	invalidDirection := validAnalysisOutput()
	invalidDirection.SuggestedDirection = "fix"
	if _, _, err := validateAnalysisOutput(invalidDirection, content); !errors.Is(err, ai.ErrInvalidResponse) {
		t.Errorf("invalid direction error = %v, want ErrInvalidResponse", err)
	}
}

func TestParseAnalysisResponse(t *testing.T) {
	valid := `{"summary":"S","observation":"O","interpretation":"I","confidence":0.7,"reflection_question":"Q?","suggested_direction":"improve","evidence_note":"N"}`
	output, err := parseAnalysisResponse("```json\n" + valid + "\n```")
	if err != nil {
		t.Fatalf("parse fenced analysis: %v", err)
	}
	if output.Summary != "S" {
		t.Errorf("summary = %q", output.Summary)
	}
	if _, err := parseAnalysisResponse("not json"); !errors.Is(err, ai.ErrInvalidResponse) {
		t.Errorf("invalid JSON error = %v, want ErrInvalidResponse", err)
	}
}
