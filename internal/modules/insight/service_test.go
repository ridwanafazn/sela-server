package insight

import (
	"testing"
	"time"
)

func TestResolvePeriod(t *testing.T) {
	location := time.UTC
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)

	weekly, err := resolvePeriod(GenerateInput{Period: GenerationWeekly}, location, now)
	if err != nil {
		t.Fatalf("weekly: %v", err)
	}
	if weekly.start.Format("2006-01-02") != "2026-11-09" ||
		weekly.end.Format("2006-01-02") != "2026-11-15" ||
		weekly.generationType != GenerationWeekly {
		t.Errorf("weekly period = %+v", weekly)
	}

	monthly, err := resolvePeriod(GenerateInput{Period: GenerationMonthly}, location, now)
	if err != nil {
		t.Fatalf("monthly: %v", err)
	}
	if monthly.start.Format("2006-01-02") != "2026-11-01" ||
		monthly.end.Format("2006-01-02") != "2026-11-15" ||
		monthly.generationType != GenerationMonthly {
		t.Errorf("monthly period = %+v", monthly)
	}

	start, end := "2026-10-01", "2026-10-31"
	custom, err := resolvePeriod(GenerateInput{
		Period:    "custom",
		StartDate: &start,
		EndDate:   &end,
	}, location, now)
	if err != nil {
		t.Fatalf("custom: %v", err)
	}
	if custom.start.Format("2006-01-02") != start ||
		custom.end.Format("2006-01-02") != end ||
		custom.generationType != GenerationOnDemand {
		t.Errorf("custom period = %+v", custom)
	}
}

func TestResolvePeriodValidation(t *testing.T) {
	location := time.UTC
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	start, end := "2026-11-01", "2026-11-15"

	tests := []struct {
		name  string
		input GenerateInput
	}{
		{name: "custom missing dates", input: GenerateInput{Period: "custom"}},
		{name: "only one date", input: GenerateInput{Period: GenerationWeekly, StartDate: &start}},
		{name: "start after end", input: GenerateInput{Period: "custom", StartDate: &end, EndDate: &start}},
		{name: "bad start", input: GenerateInput{Period: "custom", StartDate: ptr("not-a-date"), EndDate: &end}},
		{name: "range too long", input: GenerateInput{Period: "custom", StartDate: ptr("2025-01-01"), EndDate: &end}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := resolvePeriod(test.input, location, now); err == nil {
				t.Error("expected a validation error")
			}
		})
	}
}

func TestConfidenceLabel(t *testing.T) {
	tests := []struct {
		value float64
		want  string
	}{
		{0.9, ConfidenceStrong},
		{0.75, ConfidenceStrong},
		{0.7, ConfidenceModerate},
		{0.55, ConfidenceModerate},
		{0.4, ConfidenceLimited},
	}
	for _, test := range tests {
		if got := confidenceLabel(test.value); got != test.want {
			t.Errorf("confidenceLabel(%v) = %s, want %s", test.value, got, test.want)
		}
	}
}

func ptr(value string) *string {
	return &value
}
