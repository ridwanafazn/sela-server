package privacy

import "testing"

func TestContextPolicy(t *testing.T) {
	cases := []struct {
		mode     string
		journals bool
		memories bool
	}{
		{ContextNone, false, false},
		{ContextJournals, true, false},
		{ContextJournalsAndMemories, true, true},
		{"unknown", false, false},
	}
	for _, testCase := range cases {
		journals, memories := ContextPolicy(testCase.mode)
		if journals != testCase.journals || memories != testCase.memories {
			t.Errorf("ContextPolicy(%q) = (%t, %t), want (%t, %t)",
				testCase.mode, journals, memories, testCase.journals, testCase.memories)
		}
	}
}
