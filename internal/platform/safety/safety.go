// Package safety applies deterministic distress signals and locale-specific
// crisis resources. It is an application-level safeguard, not a clinical
// assessment.
package safety

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

//go:embed resources.json
var embeddedResources []byte

// Level is an application-level safety signal.
type Level string

const (
	LevelNone     Level = "none"
	LevelElevated Level = "elevated"
	LevelCrisis   Level = "crisis"
)

// Assessment is the deterministic safety signal for one message.
type Assessment struct {
	Level    Level
	Category string
}

// Resource is one crisis support entry.
type Resource struct {
	Name        string `json:"name"`
	Contact     string `json:"contact"`
	Description string `json:"description"`
}

// Locale is a locale-specific crisis resource set.
type Locale struct {
	Language  string     `json:"language"`
	Emergency string     `json:"emergency"`
	Reviewed  bool       `json:"reviewed"`
	Notes     string     `json:"notes"`
	Resources []Resource `json:"resources"`
}

type resourceFile struct {
	Version string            `json:"version"`
	Locales map[string]Locale `json:"locales"`
}

// Service classifies messages and renders reviewed-format escalation text.
type Service struct {
	file resourceFile
}

// NewService loads crisis resources from path, or the embedded reviewable
// defaults when path is empty.
func NewService(path string) (*Service, error) {
	raw := embeddedResources
	if strings.TrimSpace(path) != "" {
		loaded, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read safety resources: %w", err)
		}
		raw = loaded
	}

	var file resourceFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("decode safety resources: %w", err)
	}
	if file.Version == "" {
		return nil, fmt.Errorf("safety resources version is required")
	}
	if _, ok := file.Locales["default"]; !ok {
		return nil, fmt.Errorf("safety resources must include a default locale")
	}
	for code, locale := range file.Locales {
		if locale.Emergency == "" {
			return nil, fmt.Errorf("safety locale %s is missing an emergency contact", code)
		}
	}
	return &Service{file: file}, nil
}

// Version returns the configured resource bundle version.
func (s *Service) Version() string {
	return s.file.Version
}

// UnreviewedLocales returns locale codes that still need local review before
// production use.
func (s *Service) UnreviewedLocales() []string {
	var codes []string
	for code, locale := range s.file.Locales {
		if !locale.Reviewed {
			codes = append(codes, code)
		}
	}
	return codes
}

// Locale returns the best matching locale for a profile language and timezone.
func (s *Service) Locale(language, timezone string) Locale {
	if s == nil {
		return Locale{}
	}
	lang := strings.ToLower(strings.TrimSpace(language))
	base := strings.SplitN(lang, "-", 2)[0]
	tz := strings.ToLower(strings.TrimSpace(timezone))

	if base == "id" {
		if locale, ok := s.file.Locales["id-ID"]; ok {
			return locale
		}
	}
	if base == "en" {
		if strings.HasPrefix(tz, "america/") {
			if locale, ok := s.file.Locales["en-US"]; ok {
				return locale
			}
		}
		if locale, ok := s.file.Locales["en"]; ok {
			return locale
		}
	}
	return s.file.Locales["default"]
}

// Assess classifies explicit self-harm, suicide, or immediate danger as crisis
// and hopelessness or severe distress as elevated.
func Assess(text string) Assessment {
	normalized := normalize(text)
	for _, phrase := range safeNegations {
		normalized = strings.ReplaceAll(normalized, phrase, "")
	}

	for _, phrase := range crisisPhrases {
		if strings.Contains(normalized, phrase) {
			return Assessment{Level: LevelCrisis, Category: "self_harm_or_suicide"}
		}
	}
	for _, phrase := range immediateDangerPhrases {
		if strings.Contains(normalized, phrase) {
			return Assessment{Level: LevelCrisis, Category: "immediate_danger"}
		}
	}
	for _, phrase := range elevatedPhrases {
		if strings.Contains(normalized, phrase) {
			return Assessment{Level: LevelElevated, Category: "severe_distress"}
		}
	}
	return Assessment{Level: LevelNone}
}

// EscalationMessage renders a bounded, locale-specific safety response.
func (s *Service) EscalationMessage(language, timezone string) string {
	locale := s.Locale(language, timezone)
	if locale.Language == "id" {
		return indonesianEscalation(locale)
	}
	return englishEscalation(locale)
}

// ElevatedInstruction is injected into the Companion prompt for non-crisis
// serious distress.
func (s *Service) ElevatedInstruction(language, timezone, category string) string {
	locale := s.Locale(language, timezone)
	var builder strings.Builder
	builder.WriteString("The current user message contains signs of serious distress")
	if category != "" {
		builder.WriteString(" (" + category + ")")
	}
	builder.WriteString(". Prioritize safety, validate feelings without diagnosis, and encourage contacting a trusted person or professional support. ")
	builder.WriteString("Surface these locale-specific resources when relevant: ")
	for _, resource := range locale.Resources {
		builder.WriteString(resource.Name + " (" + resource.Contact + "); ")
	}
	builder.WriteString("Do not continue ordinary reflection prompts as if nothing happened.")
	return builder.String()
}

// EnsureElevatedFooter appends a bounded resource footer when an elevated
// response does not already mention a configured resource or emergency contact.
func (s *Service) EnsureElevatedFooter(content, language, timezone string) string {
	locale := s.Locale(language, timezone)
	if strings.Contains(content, locale.Emergency) {
		return content
	}
	for _, resource := range locale.Resources {
		if strings.Contains(content, resource.Name) || strings.Contains(content, resource.Contact) {
			return content
		}
	}

	var footer strings.Builder
	footer.WriteString("\n\n")
	if locale.Language == "id" {
		footer.WriteString("Kalau rasa berat ini berlanjut, kamu bisa menghubungi ")
		for index, resource := range locale.Resources {
			if index > 0 {
				footer.WriteString(", ")
			}
			footer.WriteString(resource.Name + " (" + resource.Contact + ")")
		}
		footer.WriteString(". Jika ada bahaya langsung, hubungi " + locale.Emergency + ".")
	} else {
		footer.WriteString("If this distress continues, you can contact ")
		for index, resource := range locale.Resources {
			if index > 0 {
				footer.WriteString(", ")
			}
			footer.WriteString(resource.Name + " (" + resource.Contact + ")")
		}
		footer.WriteString(". If there is immediate danger, contact " + locale.Emergency + ".")
	}
	return content + footer.String()
}

func indonesianEscalation(locale Locale) string {
	var builder strings.Builder
	builder.WriteString("Aku perlu berhenti dari refleksi biasa untuk sementara karena yang kamu tulis terdengar serius, dan keselamatanmu lebih penting.\n\n")
	builder.WriteString("Jika kamu sedang dalam bahaya langsung atau berpikir untuk menyakiti diri, hubungi layanan darurat sekarang: " + locale.Emergency + ".\n\n")
	builder.WriteString("Kamu juga bisa menghubungi:\n")
	for _, resource := range locale.Resources {
		builder.WriteString("- " + resource.Name + ": " + resource.Contact)
		if resource.Description != "" {
			builder.WriteString(" — " + resource.Description)
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\nJika memungkinkan, hubungi orang yang kamu percaya dan usahakan tidak sendirian. Aku di sini untuk menemani, tetapi aku bukan layanan darurat dan tidak bisa menggantikan bantuan profesional.")
	return builder.String()
}

func englishEscalation(locale Locale) string {
	var builder strings.Builder
	builder.WriteString("I need to pause ordinary reflection because what you wrote sounds serious, and your safety matters most.\n\n")
	builder.WriteString("If you are in immediate danger or thinking about harming yourself, contact local emergency services now: " + locale.Emergency + ".\n\n")
	builder.WriteString("You can also contact:\n")
	for _, resource := range locale.Resources {
		builder.WriteString("- " + resource.Name + ": " + resource.Contact)
		if resource.Description != "" {
			builder.WriteString(" — " + resource.Description)
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\nIf possible, contact someone you trust and try not to be alone. I can stay with you here, but I am not an emergency service and cannot replace professional care.")
	return builder.String()
}

func normalize(text string) string {
	return strings.Join(strings.Fields(strings.ToLower(text)), " ")
}

var safeNegations = []string{
	"do not want to die",
	"don't want to die",
	"not suicidal",
	"tidak ingin mati",
	"nggak mau mati",
	"tidak mau bunuh diri",
	"tidak akan menyakiti diri",
}

var crisisPhrases = []string{
	"kill myself",
	"killing myself",
	"suicide",
	"suicidal",
	"end my life",
	"take my own life",
	"want to die",
	"wanna die",
	"don't want to live",
	"do not want to live",
	"better off dead",
	"self-harm",
	"self harm",
	"hurt myself",
	"cut myself",
	"overdose",
	"took pills",
	"jump off",
	"hang myself",
	"bunuh diri",
	"mengakhiri hidup",
	"akhiri hidup",
	"ingin mati",
	"pengen mati",
	"mau mati",
	"tidak ingin hidup",
	"nggak mau hidup",
	"lebih baik mati",
	"menyakiti diri",
	"melukai diri",
}

var immediateDangerPhrases = []string{
	"sedang dalam bahaya",
	"immediate danger",
	"about to hurt myself",
	"already took",
	"sudah minum pil",
	"minum obat berlebihan",
	"overdosis",
}

var elevatedPhrases = []string{
	"hidup ini kosong",
	"hidupku kosong",
	"tidak ada gunanya hidup",
	"nggak ada gunanya",
	"putus asa",
	"hopeless",
	"no reason to live",
	"nothing matters",
	"worthless",
	"capek hidup",
	"lelah hidup",
	"benci hidup",
	"tidak kuat lagi",
	"can't go on",
	"cannot go on",
	"give up",
	"menyerah",
}
