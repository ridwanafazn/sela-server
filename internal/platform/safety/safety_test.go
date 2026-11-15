package safety

import (
	"strings"
	"testing"
)

func TestAssess(t *testing.T) {
	tests := []struct {
		name string
		text string
		want Level
	}{
		{name: "benign", text: "Aku capek kerja tapi mau istirahat.", want: LevelNone},
		{name: "negated", text: "Aku tidak ingin mati, aku hanya butuh bantuan.", want: LevelNone},
		{name: "english negation", text: "I don't want to die, I want help.", want: LevelNone},
		{name: "indonesian crisis", text: "Aku ingin mati dan sudah menyakiti diri.", want: LevelCrisis},
		{name: "english crisis", text: "I want to kill myself tonight.", want: LevelCrisis},
		{name: "immediate danger", text: "Aku sedang dalam bahaya dan sudah minum pil.", want: LevelCrisis},
		{name: "hopelessness", text: "Hidup ini kosong dan tidak ada gunanya.", want: LevelElevated},
		{name: "english hopelessness", text: "I feel hopeless and worthless.", want: LevelElevated},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Assess(test.text)
			if got.Level != test.want {
				t.Errorf("Assess(%q) = %s, want %s", test.text, got.Level, test.want)
			}
		})
	}
}

func TestLocaleSelection(t *testing.T) {
	service, err := NewService("")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	tests := []struct {
		language string
		timezone string
		want     string
	}{
		{"id", "Asia/Jakarta", "id-ID"},
		{"id-ID", "Asia/Makassar", "id-ID"},
		{"en", "America/New_York", "en-US"},
		{"en-GB", "Europe/London", "en"},
		{"fr", "Europe/Paris", "default"},
	}
	for _, test := range tests {
		locale := service.Locale(test.language, test.timezone)
		if locale.Language == "" {
			t.Errorf("locale for %s/%s is empty", test.language, test.timezone)
		}
		if test.want == "id-ID" && !strings.Contains(locale.Emergency, "112") {
			t.Errorf("id locale emergency = %q", locale.Emergency)
		}
	}
}

func TestElevatedFooter(t *testing.T) {
	service, err := NewService("")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	withFooter := service.EnsureElevatedFooter("Aku di sini.", "id", "Asia/Jakarta")
	for _, expected := range []string{"SEJIWA", "119", "bahaya langsung"} {
		if !strings.Contains(withFooter, expected) {
			t.Errorf("footer is missing %q: %s", expected, withFooter)
		}
	}

	alreadySafe := "Hubungi SEJIWA 119 ekstensi 8 sekarang."
	if got := service.EnsureElevatedFooter(alreadySafe, "id", "Asia/Jakarta"); got != alreadySafe {
		t.Errorf("footer was appended despite an existing resource: %s", got)
	}
}

func TestEscalationMessageContainsResources(t *testing.T) {
	service, err := NewService("")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	message := service.EscalationMessage("id", "Asia/Jakarta")
	for _, expected := range []string{"112", "119", "SEJIWA", "layanan darurat", "bukan layanan darurat"} {
		if !strings.Contains(message, expected) {
			t.Errorf("message is missing %q: %s", expected, message)
		}
	}
	if strings.Contains(message, "Satu pertanyaan") {
		t.Error("escalation message contains an ordinary reflection prompt")
	}
}
