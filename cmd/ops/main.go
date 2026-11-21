// Command ops provides local beta-hardening checks without a hosted metrics
// backend: health gates, AI usage summaries, and stale-memory counts.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ridwanafazn/sela-server/internal/config"
	"github.com/ridwanafazn/sela-server/internal/platform/database"
	"github.com/ridwanafazn/sela-server/migrations"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "ops: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ops <health|usage|stale-memories|user|grant|revoke>")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer func() {
		_ = db.Close()
	}()

	switch args[0] {
	case "health":
		return healthCommand(ctx, db, cfg, args[1:])
	case "usage":
		return usageCommand(ctx, db, args[1:])
	case "stale-memories":
		return staleMemoriesCommand(ctx, db, args[1:])
	case "user":
		return userCommand(ctx, db, args[1:])
	case "grant":
		return grantCommand(ctx, db, args[1:])
	case "revoke":
		return revokeCommand(ctx, db, args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

type healthReport struct {
	Status            string   `json:"status"`
	MigrationVersion  uint     `json:"migration_version"`
	MigrationExpected uint     `json:"migration_expected"`
	MigrationDirty    bool     `json:"migration_dirty"`
	PendingJobs       int      `json:"pending_jobs"`
	FailedJobs24h     int      `json:"failed_jobs_24h"`
	OldestPendingMin  float64  `json:"oldest_pending_minutes"`
	DueSchedules      int      `json:"due_schedules"`
	AICalls24h        int      `json:"ai_calls_24h"`
	AIFailures24h     int      `json:"ai_failures_24h"`
	MonthlyCostUSD    float64  `json:"monthly_cost_usd"`
	MonthlyBudgetUSD  float64  `json:"monthly_budget_usd"`
	Problems          []string `json:"problems"`
	Warnings          []string `json:"warnings"`
}

func healthCommand(ctx context.Context, db *database.DB, cfg *config.Config, args []string) error {
	flags := flag.NewFlagSet("health", flag.ContinueOnError)
	asJSON := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}

	expected, err := migrations.LatestVersion()
	if err != nil {
		return err
	}

	report := healthReport{Status: "ok", MigrationExpected: expected}
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT version, dirty FROM schema_migrations LIMIT 1`,
	).Scan(&report.MigrationVersion, &report.MigrationDirty); err != nil {
		return fmt.Errorf("read migration version: %w", err)
	}

	if err := db.SQL.QueryRowContext(ctx,
		`SELECT
		   count(*) FILTER (WHERE status IN ('pending', 'retry')),
		   count(*) FILTER (WHERE status = 'failed' AND updated_at >= now() - interval '24 hours'),
		   COALESCE(EXTRACT(EPOCH FROM (now() - min(run_at) FILTER (WHERE status IN ('pending', 'retry')))) / 60, 0)
		 FROM scheduled_jobs`,
	).Scan(&report.PendingJobs, &report.FailedJobs24h, &report.OldestPendingMin); err != nil {
		return fmt.Errorf("read job health: %w", err)
	}

	if err := db.SQL.QueryRowContext(ctx,
		`SELECT count(*) FROM insight_schedules WHERE enabled = true AND next_run_at <= now()`,
	).Scan(&report.DueSchedules); err != nil {
		return fmt.Errorf("read due schedules: %w", err)
	}

	if err := db.SQL.QueryRowContext(ctx,
		`SELECT count(*),
		        count(*) FILTER (WHERE NOT success),
		        COALESCE(SUM(estimated_cost_usd) FILTER (WHERE created_at >= date_trunc('month', now())), 0)
		 FROM ai_usage_events
		 WHERE created_at >= now() - interval '24 hours'`,
	).Scan(&report.AICalls24h, &report.AIFailures24h, &report.MonthlyCostUSD); err != nil {
		return fmt.Errorf("read ai usage health: %w", err)
	}
	report.MonthlyBudgetUSD = cfg.AI.GlobalMonthlyBudgetUSD

	if report.MigrationVersion != expected {
		report.Problems = append(report.Problems, "migration version is not current")
	}
	if report.MigrationDirty {
		report.Problems = append(report.Problems, "migration state is dirty")
	}
	if report.OldestPendingMin > 10 {
		report.Problems = append(report.Problems, "job backlog older than 10 minutes")
	}
	if report.DueSchedules > 0 {
		report.Problems = append(report.Problems, "scheduled insights are due; scheduler is behind")
	}
	if report.FailedJobs24h > 0 {
		report.Warnings = append(report.Warnings, "failed jobs in the last 24 hours")
	}
	if report.AICalls24h > 0 && float64(report.AIFailures24h)/float64(report.AICalls24h) > 0.05 {
		report.Warnings = append(report.Warnings, "AI failure rate above 5% in the last 24 hours")
	}
	if report.MonthlyBudgetUSD > 0 && report.MonthlyCostUSD >= report.MonthlyBudgetUSD*0.8 {
		report.Warnings = append(report.Warnings, "global AI budget is above 80%")
	}
	if len(report.Problems) > 0 {
		report.Status = "fail"
	} else if len(report.Warnings) > 0 {
		report.Status = "warn"
	}

	if *asJSON {
		encoded, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(encoded))
	} else {
		fmt.Printf("status=%s migration=%d/%d dirty=%t\n", report.Status, report.MigrationVersion, report.MigrationExpected, report.MigrationDirty)
		fmt.Printf("jobs pending=%d failed24h=%d oldest_pending_min=%.1f due_schedules=%d\n", report.PendingJobs, report.FailedJobs24h, report.OldestPendingMin, report.DueSchedules)
		fmt.Printf("ai calls24h=%d failures24h=%d monthly_cost_usd=%.4f budget_usd=%.2f\n", report.AICalls24h, report.AIFailures24h, report.MonthlyCostUSD, report.MonthlyBudgetUSD)
		for _, problem := range report.Problems {
			fmt.Println("FAIL", problem)
		}
		for _, warning := range report.Warnings {
			fmt.Println("WARN", warning)
		}
	}

	if report.Status == "fail" {
		return fmt.Errorf("health checks failed")
	}
	return nil
}

type usageRow struct {
	Day          string  `json:"day"`
	Operation    string  `json:"operation"`
	Calls        int     `json:"calls"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	Failures     int     `json:"failures"`
}

func usageCommand(ctx context.Context, db *database.DB, args []string) error {
	flags := flag.NewFlagSet("usage", flag.ContinueOnError)
	days := flags.Int("days", 30, "lookback days")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *days < 1 || *days > 365 {
		return fmt.Errorf("days must be between 1 and 365")
	}

	rows, err := db.SQL.QueryContext(ctx,
		`SELECT to_char(date_trunc('day', created_at), 'YYYY-MM-DD') AS day,
		        operation,
		        count(*),
		        COALESCE(SUM(input_tokens), 0),
		        COALESCE(SUM(output_tokens), 0),
		        COALESCE(SUM(estimated_cost_usd), 0),
		        count(*) FILTER (WHERE NOT success)
		 FROM ai_usage_events
		 WHERE created_at >= now() - ($1 * interval '1 day')
		 GROUP BY 1, 2
		 ORDER BY 1 DESC, 2`,
		*days,
	)
	if err != nil {
		return fmt.Errorf("read ai usage: %w", err)
	}
	defer rows.Close()

	var result []usageRow
	for rows.Next() {
		var row usageRow
		if err := rows.Scan(&row.Day, &row.Operation, &row.Calls, &row.InputTokens, &row.OutputTokens, &row.CostUSD, &row.Failures); err != nil {
			return err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if *asJSON {
		encoded, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(encoded))
		return nil
	}

	if len(result) == 0 {
		fmt.Println("no AI usage events in the requested window")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "DAY\tOPERATION\tCALLS\tINPUT\tOUTPUT\tCOST_USD\tFAIL")
	for _, row := range result {
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%.6f\t%d\n", row.Day, row.Operation, row.Calls, row.InputTokens, row.OutputTokens, row.CostUSD, row.Failures)
	}
	return w.Flush()
}

type userRow struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	Status          string     `json:"status"`
	EmailVerifiedAt *time.Time `json:"email_verified_at"`
	DisplayName     *string    `json:"display_name"`
	Plan            string     `json:"plan"`
	Entitlement     string     `json:"entitlement_status"`
	PeriodEnd       *time.Time `json:"current_period_end"`
}

func userCommand(ctx context.Context, db *database.DB, args []string) error {
	flags := flag.NewFlagSet("user", flag.ContinueOnError)
	email := flags.String("email", "", "user email")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*email) == "" {
		return fmt.Errorf("email is required")
	}

	var row userRow
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT u.id, u.email, u.status, u.email_verified_at, p.display_name,
		        COALESCE(e.plan, 'free'), COALESCE(e.status, 'active'), e.current_period_end
		 FROM users u
		 JOIN profiles p ON p.user_id = u.id
		 LEFT JOIN user_entitlements e ON e.user_id = u.id
		 WHERE u.email = $1`,
		strings.ToLower(strings.TrimSpace(*email)),
	).Scan(&row.ID, &row.Email, &row.Status, &row.EmailVerifiedAt, &row.DisplayName,
		&row.Plan, &row.Entitlement, &row.PeriodEnd); err != nil {
		return fmt.Errorf("load user: %w", err)
	}

	if *asJSON {
		encoded, _ := json.MarshalIndent(row, "", "  ")
		fmt.Println(string(encoded))
		return nil
	}
	fmt.Printf("id=%s email=%s status=%s email_verified=%t plan=%s entitlement=%s\n",
		row.ID, row.Email, row.Status, row.EmailVerifiedAt != nil, row.Plan, row.Entitlement)
	if row.DisplayName != nil {
		fmt.Printf("display_name=%s\n", *row.DisplayName)
	}
	if row.PeriodEnd != nil {
		fmt.Printf("current_period_end=%s\n", row.PeriodEnd.Format(time.RFC3339))
	}
	return nil
}

func grantCommand(ctx context.Context, db *database.DB, args []string) error {
	flags := flag.NewFlagSet("grant", flag.ContinueOnError)
	email := flags.String("email", "", "user email")
	plan := flags.String("plan", "premium", "free or premium")
	days := flags.Int("days", 30, "entitlement duration in days")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*email) == "" {
		return fmt.Errorf("email is required")
	}
	if *plan != "free" && *plan != "premium" {
		return fmt.Errorf("plan must be free or premium")
	}
	if *days < 0 || *days > 3650 {
		return fmt.Errorf("days must be between 0 and 3650")
	}

	var userID string
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT id FROM users WHERE email = $1`,
		strings.ToLower(strings.TrimSpace(*email)),
	).Scan(&userID); err != nil {
		return fmt.Errorf("find user: %w", err)
	}

	now := time.Now().UTC()
	var periodEnd *time.Time
	if *days > 0 {
		value := now.AddDate(0, 0, *days)
		periodEnd = &value
	}
	if err := db.Gorm.WithContext(ctx).Exec(
		`INSERT INTO user_entitlements
		 (user_id, plan, status, current_period_end, created_at, updated_at)
		 VALUES (?, ?, 'active', ?, ?, ?)
		 ON CONFLICT (user_id) DO UPDATE SET
		   plan = EXCLUDED.plan,
		   status = EXCLUDED.status,
		   current_period_end = EXCLUDED.current_period_end,
		   updated_at = EXCLUDED.updated_at`,
		userID, *plan, periodEnd, now, now,
	).Error; err != nil {
		return fmt.Errorf("grant entitlement: %w", err)
	}
	fmt.Printf("entitlement updated: email=%s plan=%s days=%d\n", *email, *plan, *days)
	return nil
}

func revokeCommand(ctx context.Context, db *database.DB, args []string) error {
	flags := flag.NewFlagSet("revoke", flag.ContinueOnError)
	email := flags.String("email", "", "user email")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*email) == "" {
		return fmt.Errorf("email is required")
	}

	var userID string
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT id FROM users WHERE email = $1`,
		strings.ToLower(strings.TrimSpace(*email)),
	).Scan(&userID); err != nil {
		return fmt.Errorf("find user: %w", err)
	}
	now := time.Now().UTC()
	if err := db.Gorm.WithContext(ctx).Exec(
		`INSERT INTO user_entitlements
		 (user_id, plan, status, current_period_end, created_at, updated_at)
		 VALUES (?, 'free', 'active', NULL, ?, ?)
		 ON CONFLICT (user_id) DO UPDATE SET
		   plan = 'free',
		   status = 'active',
		   current_period_end = NULL,
		   updated_at = EXCLUDED.updated_at`,
		userID, now, now,
	).Error; err != nil {
		return fmt.Errorf("revoke entitlement: %w", err)
	}
	fmt.Printf("entitlement revoked: email=%s plan=free\n", *email)
	return nil
}

func staleMemoriesCommand(ctx context.Context, db *database.DB, args []string) error {
	flags := flag.NewFlagSet("stale-memories", flag.ContinueOnError)
	days := flags.Int("days", 180, "staleness threshold in days")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *days < 30 || *days > 730 {
		return fmt.Errorf("days must be between 30 and 730")
	}

	var count int
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT count(*)
		 FROM personal_memories
		 WHERE status = 'active' AND deleted_at IS NULL
		   AND COALESCE(last_confirmed_at, updated_at) < now() - ($1 * interval '1 day')`,
		*days,
	).Scan(&count); err != nil {
		return fmt.Errorf("count stale memories: %w", err)
	}

	if *asJSON {
		encoded, _ := json.MarshalIndent(map[string]any{
			"days":  *days,
			"count": count,
		}, "", "  ")
		fmt.Println(string(encoded))
		return nil
	}
	fmt.Printf("stale_active_memories=%d threshold_days=%d\n", count, *days)
	return nil
}
