package insight

import (
	"testing"
	"time"
)

func TestNextRunWeeklyAndMonthly(t *testing.T) {
	location, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	now := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC) // Saturday 17:00 Jakarta

	weekly := nextRun(now, location, CadenceWeekly)
	if got := weekly.In(location).Format("2006-01-02 15:04"); got != "2026-11-23 01:00" {
		t.Errorf("weekly next run = %s, want 2026-11-23 01:00", got)
	}

	monthly := nextRun(now, location, CadenceMonthly)
	if got := monthly.In(location).Format("2006-01-02 15:04"); got != "2026-12-01 01:00" {
		t.Errorf("monthly next run = %s, want 2026-12-01 01:00", got)
	}

	beforeFirst := time.Date(2026, 11, 30, 17, 30, 0, 0, time.UTC) // 2026-12-01 00:30 Jakarta
	monthly = nextRun(beforeFirst, location, CadenceMonthly)
	if got := monthly.In(location).Format("2006-01-02 15:04"); got != "2026-12-01 01:00" {
		t.Errorf("same-day monthly run = %s, want 2026-12-01 01:00", got)
	}
}

func TestScheduledPeriod(t *testing.T) {
	location, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	weeklyRun := time.Date(2026, 11, 23, 1, 0, 0, 0, location)
	start, end := scheduledPeriod(CadenceWeekly, weeklyRun, location)
	if start != "2026-11-16" || end != "2026-11-22" {
		t.Errorf("weekly period = %s..%s, want 2026-11-16..2026-11-22", start, end)
	}

	monthlyRun := time.Date(2026, 12, 1, 1, 0, 0, 0, location)
	start, end = scheduledPeriod(CadenceMonthly, monthlyRun, location)
	if start != "2026-11-01" || end != "2026-11-30" {
		t.Errorf("monthly period = %s..%s, want 2026-11-01..2026-11-30", start, end)
	}
}

func TestScheduleJobKeyIsStable(t *testing.T) {
	first := scheduleJobKey("11111111-1111-1111-1111-111111111111", "2026-11-16", "2026-11-22")
	second := scheduleJobKey("11111111-1111-1111-1111-111111111111", "2026-11-16", "2026-11-22")
	if first != second {
		t.Errorf("job key is not stable: %s != %s", first, second)
	}
	other := scheduleJobKey("11111111-1111-1111-1111-111111111111", "2026-11-23", "2026-11-29")
	if first == other {
		t.Error("different periods produced the same job key")
	}
}
