package memory

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/ridwanafazn/sela-server/internal/modules/ai"
)

func TestParseExtractionResponse(t *testing.T) {
	valid := `{"memories":[{"category":"preference","content":"The user prefers concise explanations.","confidence":0.9,"certainty_type":"explicit","source_ids":["11111111-1111-1111-1111-111111111111"],"sensitivity":"normal","conflicts_with":[]}]}`

	tests := []struct {
		name    string
		content string
		want    int
		wantErr bool
	}{
		{name: "valid", content: valid, want: 1},
		{name: "fenced", content: "```json\n" + valid + "\n```", want: 1},
		{name: "empty object", content: `{}`, want: 0},
		{name: "empty memories", content: `{"memories":[]}`, want: 0},
		{name: "empty", content: "  ", wantErr: true},
		{name: "invalid json", content: "not json", wantErr: true},
		{name: "wrong shape", content: `{"memories":"nope"}`, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			memories, err := parseExtractionResponse(test.content)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				if !errors.Is(err, ai.ErrInvalidResponse) {
					t.Errorf("error = %v, want ErrInvalidResponse", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseExtractionResponse: %v", err)
			}
			if len(memories) != test.want {
				t.Errorf("memories = %d, want %d", len(memories), test.want)
			}
		})
	}
}

func TestNormalizeCategory(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{raw: "Preference", want: "preference"},
		{raw: "Work Stress", want: "work_stress"},
		{raw: "  health  ", want: "health"},
		{raw: "relationship/family", want: "relationshipfamily"},
		{raw: "", want: ""},
		{raw: "!!!", want: ""},
	}

	for _, test := range tests {
		got := normalizeCategory(test.raw)
		if test.want == "" {
			if got != nil {
				t.Errorf("normalizeCategory(%q) = %q, want nil", test.raw, *got)
			}
			continue
		}
		if got == nil || *got != test.want {
			t.Errorf("normalizeCategory(%q) = %v, want %q", test.raw, got, test.want)
		}
	}
}

func TestValidateCandidateRejectsUnsafeOutput(t *testing.T) {
	entry := &JournalSource{
		ID:      "11111111-1111-1111-1111-111111111111",
		Content: "I could not sleep well because of work.",
	}
	service := &ExtractionService{repo: &fakeExtractionRepository{
		eligible: map[string]bool{entry.ID: true},
	}}
	confidence := 0.8

	base := extractedMemory{
		Category:      "work",
		Content:       "The user finds work stressful.",
		Confidence:    &confidence,
		CertaintyType: "explicit",
		SourceIDs:     []string{entry.ID},
		Sensitivity:   "normal",
	}

	tests := []struct {
		name      string
		mutate    func(*extractedMemory)
		sensitive bool
	}{
		{name: "valid shape", mutate: func(*extractedMemory) {}},
		{name: "sensitive", mutate: func(value *extractedMemory) { value.Sensitivity = "sensitive" }, sensitive: true},
		{name: "unknown sensitivity", mutate: func(value *extractedMemory) { value.Sensitivity = "private" }},
		{name: "unknown certainty", mutate: func(value *extractedMemory) { value.CertaintyType = "guess" }},
		{name: "missing confidence", mutate: func(value *extractedMemory) { value.Confidence = nil }},
		{name: "confidence too high", mutate: func(value *extractedMemory) { high := 1.5; value.Confidence = &high }},
		{name: "negative confidence", mutate: func(value *extractedMemory) { low := -0.1; value.Confidence = &low }},
		{name: "empty content", mutate: func(value *extractedMemory) { value.Content = "  " }},
		{name: "copied journal", mutate: func(value *extractedMemory) { value.Content = entry.Content }},
		{name: "too long", mutate: func(value *extractedMemory) { value.Content = strings.Repeat("a", 501) }},
		{name: "no sources", mutate: func(value *extractedMemory) { value.SourceIDs = nil }},
		{name: "two sources", mutate: func(value *extractedMemory) {
			value.SourceIDs = []string{entry.ID, "22222222-2222-2222-2222-222222222222"}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			test.mutate(&candidate)

			_, accepted, sensitive, err := service.validateCandidate(t.Context(), entry, candidate)
			if err != nil {
				t.Fatalf("validateCandidate: %v", err)
			}
			if sensitive != test.sensitive {
				t.Errorf("sensitive = %v, want %v", sensitive, test.sensitive)
			}
			if test.name == "valid shape" {
				if !accepted {
					t.Error("valid candidate was rejected")
				}
				return
			}
			if accepted {
				t.Errorf("invalid candidate was accepted")
			}
		})
	}
}

func TestValidateCandidateRejectsInventedSource(t *testing.T) {
	entry := &JournalSource{
		ID:      "11111111-1111-1111-1111-111111111111",
		Content: "A calm walk by the river helped me reset.",
	}
	service := &ExtractionService{repo: &fakeExtractionRepository{
		eligible: map[string]bool{entry.ID: true},
	}}
	confidence := 0.7

	candidate := extractedMemory{
		Category:      "routine",
		Content:       "Walks help the user reset.",
		Confidence:    &confidence,
		CertaintyType: "inferred",
		SourceIDs:     []string{"22222222-2222-2222-2222-222222222222"},
		Sensitivity:   "normal",
	}

	_, accepted, _, err := service.validateCandidate(t.Context(), entry, candidate)
	if err != nil {
		t.Fatalf("validateCandidate: %v", err)
	}
	if accepted {
		t.Error("candidate with a source that was not provided was accepted")
	}
}

func TestRankMemoriesExcludesIneligibleStatusesAndPrioritizesActive(t *testing.T) {
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	candidates := []RetrievedMemory{
		{
			ID:         "11111111-1111-1111-1111-111111111111",
			Content:    "The user finds work stressful.",
			Status:     StatusCandidate,
			Confidence: 0.6,
			UpdatedAt:  now,
		},
		{
			ID:         "22222222-2222-2222-2222-222222222222",
			Content:    "The user finds work stressful.",
			Status:     StatusActive,
			Confidence: 0.6,
			UpdatedAt:  now.Add(-time.Hour),
		},
		{
			ID:         "33333333-3333-3333-3333-333333333333",
			Content:    "The user finds work stressful.",
			Status:     StatusRejected,
			Confidence: 0.9,
			UpdatedAt:  now,
		},
		{
			ID:         "44444444-4444-4444-4444-444444444444",
			Content:    "The user finds work stressful.",
			Status:     StatusSuperseded,
			Confidence: 0.9,
			UpdatedAt:  now,
		},
		{
			ID:         "55555555-5555-5555-5555-555555555555",
			Content:    "The user finds work stressful.",
			Status:     StatusDeleted,
			Confidence: 0.9,
			UpdatedAt:  now,
		},
		{
			ID:         "66666666-6666-6666-6666-666666666666",
			Content:    "The user enjoys cooking pasta.",
			Status:     StatusActive,
			Confidence: 0.9,
			UpdatedAt:  now,
		},
	}

	results := rankMemories(candidates, []string{"work"}, 5)
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if results[0].Status != StatusActive {
		t.Errorf("top memory status = %s, want active", results[0].Status)
	}
	if results[1].Status != StatusCandidate {
		t.Errorf("second memory status = %s, want candidate", results[1].Status)
	}
	for _, result := range results {
		switch result.Status {
		case StatusRejected, StatusSuperseded, StatusDeleted:
			t.Errorf("ineligible memory %s appeared in results", result.ID)
		}
	}
}

func TestRankMemoriesRespectsLimit(t *testing.T) {
	now := time.Now().UTC()
	candidates := []RetrievedMemory{
		{ID: "1", Content: "work one", Status: StatusActive, Confidence: 0.5, UpdatedAt: now},
		{ID: "2", Content: "work two", Status: StatusActive, Confidence: 0.5, UpdatedAt: now},
		{ID: "3", Content: "work three", Status: StatusActive, Confidence: 0.5, UpdatedAt: now},
	}

	results := rankMemories(candidates, []string{"work"}, 2)
	if len(results) != 2 {
		t.Errorf("results = %d, want 2", len(results))
	}
}

type fakeExtractionRepository struct {
	entry    *JournalSource
	eligible map[string]bool
	owned    map[string]bool
	existing *Memory
	created  []*Memory
	added    int
}

func (f *fakeExtractionRepository) JournalForExtraction(context.Context, string) (*JournalSource, error) {
	if f.entry == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return f.entry, nil
}

func (f *fakeExtractionRepository) EligibleJournalIDs(_ context.Context, _ string, ids []string) (map[string]bool, error) {
	result := make(map[string]bool, len(ids))
	for _, id := range ids {
		if f.eligible[id] {
			result[id] = true
		}
	}
	return result, nil
}

func (f *fakeExtractionRepository) OwnedMemoryIDs(_ context.Context, _ string, ids []string) (map[string]bool, error) {
	result := make(map[string]bool, len(ids))
	for _, id := range ids {
		if f.owned[id] {
			result[id] = true
		}
	}
	return result, nil
}

func (f *fakeExtractionRepository) FindByNormalizedContent(context.Context, string, string) (*Memory, error) {
	if f.existing == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return f.existing, nil
}

func (f *fakeExtractionRepository) AddJournalSource(context.Context, string, string, time.Time) error {
	f.added++
	return nil
}

func (f *fakeExtractionRepository) Create(_ context.Context, memory *Memory, _ string) error {
	f.created = append(f.created, memory)
	return nil
}
