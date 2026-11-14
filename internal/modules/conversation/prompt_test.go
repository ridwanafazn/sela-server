package conversation

import (
	"strings"
	"testing"
	"time"

	"github.com/ridwanafazn/sela-server/internal/modules/journal"
)

func TestBuildSystemPromptIncludesContextAndPreferences(t *testing.T) {
	title := "Late night"
	prompt, err := BuildSystemPrompt("id", "gentle", []journal.RetrievedJournal{{
		JournalID: "11111111-1111-1111-1111-111111111111",
		Title:     &title,
		Content:   "I could not sleep well and kept thinking about work.",
		EntryDate: time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC),
	}})
	if err != nil {
		t.Fatalf("BuildSystemPrompt: %v", err)
	}

	for _, expected := range []string{
		"Language: id",
		"Preferred style: gentle",
		"<retrieved_context>",
		"<journal id=",
		"I could not sleep well",
		"</retrieved_context>",
	} {
		if !strings.Contains(prompt, expected) {
			t.Errorf("prompt is missing %q", expected)
		}
	}
}

func TestBuildSystemPromptWithoutContext(t *testing.T) {
	prompt, err := BuildSystemPrompt("en", "direct", nil)
	if err != nil {
		t.Fatalf("BuildSystemPrompt: %v", err)
	}
	if !strings.Contains(prompt, "No relevant journal entries were found.") {
		t.Error("prompt does not explain the empty context")
	}
}

func TestBuildSystemPromptTruncatesContext(t *testing.T) {
	huge := strings.Repeat("long journal content ", 5000)
	prompt, err := BuildSystemPrompt("id", "adaptive", []journal.RetrievedJournal{{
		JournalID: "22222222-2222-2222-2222-222222222222",
		Content:   huge,
		EntryDate: time.Now().UTC(),
	}})
	if err != nil {
		t.Fatalf("BuildSystemPrompt: %v", err)
	}
	if len(prompt) > 30000 {
		t.Errorf("prompt length = %d, want a bounded context", len(prompt))
	}
	if !strings.Contains(prompt, "</retrieved_context>") {
		t.Error("prompt is incomplete")
	}
}

func TestBuildMessagesKeepsNewestWithinBudget(t *testing.T) {
	recent := []Message{
		{Role: RoleUser, Content: strings.Repeat("old ", 2000)},
		{Role: RoleAssistant, Content: "A short reply."},
		{Role: RoleUser, Content: "The current question."},
	}

	messages := BuildMessages(recent, 100)
	if len(messages) == 0 {
		t.Fatal("no messages selected")
	}
	last := messages[len(messages)-1]
	if last.Role != RoleUser || last.Content != "The current question." {
		t.Errorf("last message = %+v, want the current question", last)
	}
	for _, message := range messages {
		if strings.HasPrefix(message.Content, "old ") {
			t.Error("the oversized old message was included")
		}
	}
}
