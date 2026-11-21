// Package evaluation provides versioned, deterministic quality gates for
// safety and prompt-injection behavior. Live model quality evaluation remains
// a separate manual step.
package evaluation

import (
	_ "embed"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ridwanafazn/sela-server/internal/modules/conversation"
	"github.com/ridwanafazn/sela-server/internal/modules/journal"
	"github.com/ridwanafazn/sela-server/internal/modules/memory"
	"github.com/ridwanafazn/sela-server/internal/platform/safety"
)

//go:embed cases.json
var casesJSON []byte

type safetyCase struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Level string `json:"level"`
}

type injectionCase struct {
	ID            string `json:"id"`
	SourceType    string `json:"source_type"`
	Content       string `json:"content"`
	EscapedMarker string `json:"escaped_marker"`
}

type dataset struct {
	Version         string          `json:"version"`
	Safety          []safetyCase    `json:"safety"`
	PromptInjection []injectionCase `json:"prompt_injection"`
}

func loadDataset(t *testing.T) dataset {
	t.Helper()
	var data dataset
	if err := json.Unmarshal(casesJSON, &data); err != nil {
		t.Fatalf("decode evaluation dataset: %v", err)
	}
	if data.Version == "" {
		t.Fatal("evaluation dataset version is empty")
	}
	if len(data.Safety) == 0 || len(data.PromptInjection) == 0 {
		t.Fatal("evaluation dataset is missing cases")
	}
	return data
}

func TestSafetyEvaluationGate(t *testing.T) {
	data := loadDataset(t)
	passed := 0
	for _, test := range data.Safety {
		got := safety.Assess(test.Text)
		if string(got.Level) == test.Level {
			passed++
			continue
		}
		t.Errorf("case %s: level = %s, want %s", test.ID, got.Level, test.Level)
	}
	if passed != len(data.Safety) {
		t.Errorf("safety gate = %d/%d, want 100%%", passed, len(data.Safety))
	}
}

func TestCompanionPromptV2BehavioralRequirements(t *testing.T) {
	prompt, err := conversation.BuildSystemPrompt("en", "adaptive", nil, nil, "")
	if err != nil {
		t.Fatalf("build companion prompt: %v", err)
	}
	required := []string{
		"Healthy challenge",
		"Adaptive style",
		"Reflection approach",
		"not a clinician",
		"untrusted",
		"do not diagnose",
	}
	lower := strings.ToLower(prompt)
	for _, want := range required {
		if !strings.Contains(lower, strings.ToLower(want)) {
			t.Errorf("companion prompt is missing %q", want)
		}
	}
}

func TestPromptInjectionEvaluationGate(t *testing.T) {
	data := loadDataset(t)
	passed := 0

	for _, test := range data.PromptInjection {
		var (
			prompt string
			err    error
		)
		switch test.SourceType {
		case "journal":
			prompt, err = conversation.BuildSystemPrompt("en", "adaptive", []journal.RetrievedJournal{{
				JournalID: uuid.NewString(),
				Content:   test.Content,
				EntryDate: time.Now().UTC(),
			}}, nil, "")
		case "memory":
			prompt, err = conversation.BuildSystemPrompt("en", "adaptive", nil, []memory.RetrievedMemory{{
				ID:         uuid.NewString(),
				Content:    test.Content,
				Status:     memory.StatusCandidate,
				Confidence: 0.5,
			}}, "")
		default:
			t.Errorf("case %s: unknown source type %s", test.ID, test.SourceType)
			continue
		}
		if err != nil {
			t.Errorf("case %s: build prompt: %v", test.ID, err)
			continue
		}

		if strings.Contains(prompt, "<system>") && !strings.Contains(prompt, test.EscapedMarker) {
			t.Errorf("case %s: injected tag was not escaped", test.ID)
			continue
		}
		if !strings.Contains(prompt, test.EscapedMarker) {
			t.Errorf("case %s: escaped marker %q is missing", test.ID, test.EscapedMarker)
			continue
		}
		if !strings.Contains(strings.ToLower(prompt), "untrusted") {
			t.Errorf("case %s: prompt does not label retrieved content as untrusted", test.ID)
			continue
		}
		passed++
	}

	if passed != len(data.PromptInjection) {
		t.Errorf("prompt-injection gate = %d/%d, want 100%%", passed, len(data.PromptInjection))
	}
}
