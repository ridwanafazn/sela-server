package insight

import "time"

// Schedule cadences.
const (
	CadenceWeekly  = "weekly"
	CadenceMonthly = "monthly"
)

// OutcomeBudgetExhausted is returned when a scheduled insight cannot run
// because the user's AI budget is exhausted.
const OutcomeBudgetExhausted = "budget_exhausted"

// Schedule is the persisted weekly or monthly reflection schedule.
type Schedule struct {
	ID        string     `gorm:"column:id;primaryKey;type:uuid"`
	UserID    string     `gorm:"column:user_id;type:uuid"`
	Cadence   string     `gorm:"column:cadence"`
	Timezone  string     `gorm:"column:timezone"`
	Enabled   bool       `gorm:"column:enabled"`
	NextRunAt time.Time  `gorm:"column:next_run_at"`
	LastRunAt *time.Time `gorm:"column:last_run_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at"`
}

// TableName pins the table name.
func (Schedule) TableName() string {
	return "insight_schedules"
}

// ScheduleView is the API representation of a schedule.
type ScheduleView struct {
	Enabled       bool       `json:"enabled"`
	Cadence       *string    `json:"cadence"`
	Timezone      string     `json:"timezone"`
	NextRunAt     *time.Time `json:"next_run_at"`
	LastRunAt     *time.Time `json:"last_run_at"`
	BudgetBlocked bool       `json:"budget_blocked"`
	UpdatedAt     *time.Time `json:"updated_at"`
}

// ScheduleInput is a validated schedule update.
type ScheduleInput struct {
	Enabled bool
	Cadence string
}

// FanOutResult summarizes one scheduler tick.
type FanOutResult struct {
	Due      int
	Enqueued int
	Existing int
	Failed   int
}
