package conversation

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/ridwanafazn/sela-server/internal/modules/ai"
	"github.com/ridwanafazn/sela-server/internal/modules/journal"
	"github.com/ridwanafazn/sela-server/internal/modules/memory"
	"github.com/ridwanafazn/sela-server/prompts"
)

var companionTemplate = template.Must(template.New("companion").Parse(prompts.CompanionV2))

// BuildSystemPrompt renders the versioned Companion system prompt with
// retrieved memories and journal evidence.
func BuildSystemPrompt(
	language, style string,
	retrieved []journal.RetrievedJournal,
	memories []memory.RetrievedMemory,
	safetyNote string,
) (string, error) {
	var builder strings.Builder
	err := companionTemplate.Execute(&builder, struct {
		Language string
		Style    string
		Memories string
		Context  string
		Safety   string
	}{
		Language: language,
		Style:    style,
		Memories: buildMemoryContext(memories),
		Context:  buildRetrievedContext(retrieved),
		Safety:   safetyNote,
	})
	if err != nil {
		return "", fmt.Errorf("render companion prompt: %w", err)
	}
	return builder.String(), nil
}

func buildMemoryContext(memories []memory.RetrievedMemory) string {
	if len(memories) == 0 {
		return "No personal memories were retrieved."
	}

	var builder strings.Builder
	for _, entry := range memories {
		category := ""
		if entry.Category != nil {
			category = *entry.Category
		}
		fmt.Fprintf(
			&builder,
			"<memory id=%q status=%q category=%q confidence=%.2f>\n%s\n</memory>\n",
			entry.ID,
			entry.Status,
			category,
			entry.Confidence,
			escapePromptText(truncateToRunes(entry.Content, 500)),
		)
	}
	return builder.String()
}

func buildRetrievedContext(retrieved []journal.RetrievedJournal) string {
	const maxChars = 4000 * 4
	if len(retrieved) == 0 {
		return "No relevant journal entries were found."
	}

	var builder strings.Builder
	remaining := maxChars

	for _, entry := range retrieved {
		title := ""
		if entry.Title != nil {
			title = *entry.Title
		}

		header := fmt.Sprintf("<journal id=%q date=%q title=%q>\n", entry.JournalID, entry.EntryDate.Format("2006-01-02"), title)
		footer := "\n</journal>\n"
		available := remaining - len(header) - len(footer)
		if available <= 0 {
			break
		}

		content := escapePromptText(truncateToRunes(entry.Content, available))
		builder.WriteString(header)
		builder.WriteString(content)
		builder.WriteString(footer)
		remaining -= len(header) + len(content) + len(footer)
	}

	if builder.Len() == 0 {
		return "No relevant journal entries were found."
	}
	return builder.String()
}

// BuildMessages converts recent messages into provider messages within a
// token budget. Recent messages are ordered oldest to newest.
func BuildMessages(recent []Message, budgetTokens int) []ai.Message {
	if len(recent) == 0 {
		return nil
	}
	if budgetTokens <= 0 {
		budgetTokens = 1500
	}

	selected := make([]Message, 0, len(recent))
	used := 0

	for index := len(recent) - 1; index >= 0; index-- {
		message := recent[index]
		tokens := approximateTokens(message.Content)
		if used+tokens > budgetTokens && len(selected) > 0 {
			break
		}
		selected = append(selected, message)
		used += tokens
		if used >= budgetTokens {
			break
		}
	}

	messages := make([]ai.Message, 0, len(selected))
	for index := len(selected) - 1; index >= 0; index-- {
		message := selected[index]
		role := message.Role
		if role != RoleUser && role != RoleAssistant {
			continue
		}
		messages = append(messages, ai.Message{Role: role, Content: message.Content})
	}
	return messages
}

func approximateTokens(text string) int {
	tokens := len([]rune(text)) / 4
	if tokens < 1 && text != "" {
		return 1
	}
	return tokens
}

func truncateToRunes(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes])
}

func escapePromptText(text string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return replacer.Replace(text)
}
