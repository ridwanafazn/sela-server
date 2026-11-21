// Package app owns the application lifecycle: dependency wiring, the HTTP
// server, and graceful shutdown.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ridwanafazn/sela-server/internal/config"
	"github.com/ridwanafazn/sela-server/internal/modules/account"
	"github.com/ridwanafazn/sela-server/internal/modules/ai"
	"github.com/ridwanafazn/sela-server/internal/modules/auth"
	"github.com/ridwanafazn/sela-server/internal/modules/conversation"
	"github.com/ridwanafazn/sela-server/internal/modules/insight"
	"github.com/ridwanafazn/sela-server/internal/modules/job"
	"github.com/ridwanafazn/sela-server/internal/modules/journal"
	"github.com/ridwanafazn/sela-server/internal/modules/memory"
	"github.com/ridwanafazn/sela-server/internal/modules/user"
	"github.com/ridwanafazn/sela-server/internal/platform/aiusage"
	"github.com/ridwanafazn/sela-server/internal/platform/database"
	"github.com/ridwanafazn/sela-server/internal/platform/httpx"
	"github.com/ridwanafazn/sela-server/internal/platform/ratelimit"
	"github.com/ridwanafazn/sela-server/internal/platform/safety"
	"github.com/ridwanafazn/sela-server/migrations"
)

// App holds the wired application.
type App struct {
	cfg    *config.Config
	log    *slog.Logger
	db     *database.DB
	server *http.Server
}

// BuildRouter wires the HTTP routes and their dependencies.
func BuildRouter(cfg *config.Config, log *slog.Logger, db *database.DB) (*gin.Engine, error) {
	expectedVersion, err := migrations.LatestVersion()
	if err != nil {
		return nil, fmt.Errorf("read embedded migration version: %w", err)
	}

	readiness := database.NewReadiness(db, expectedVersion, cfg.Health.Timeout)
	safetyService, err := safety.NewService(cfg.Safety.ResourcesFile)
	if err != nil {
		return nil, fmt.Errorf("load safety resources: %w", err)
	}
	if unreviewed := safetyService.UnreviewedLocales(); len(unreviewed) > 0 {
		log.Warn("safety resources require local review", slog.Any("locales", unreviewed))
	}
	usageService := aiusage.NewService(
		db.Gorm,
		aiusage.Prices{
			InputPerMillion:     cfg.AI.InputCostPerMillion,
			OutputPerMillion:    cfg.AI.OutputCostPerMillion,
			EmbeddingPerMillion: cfg.AI.EmbeddingCostPerMillion,
		},
		aiusage.Budgets{
			UserMonthlyUSD:   cfg.AI.UserMonthlyBudgetUSD,
			GlobalMonthlyUSD: cfg.AI.GlobalMonthlyBudgetUSD,
		},
		log,
	)

	authRepo := auth.NewRepository(db.Gorm)
	tokens := auth.NewTokenManager(cfg.JWT.AccessSecret, cfg.JWT.Issuer, cfg.JWT.Audience, cfg.JWT.AccessTTL)
	passwords := auth.NewPasswordHasher()
	limiter := ratelimit.New(db.Gorm, cfg.RateLimit.Secret)
	authService := auth.NewService(
		authRepo,
		tokens,
		passwords,
		limiter,
		auth.Limits{
			LoginPerMinuteIP:    cfg.RateLimit.LoginPerMinuteIP,
			LoginPerHourAccount: cfg.RateLimit.LoginPerHourAccount,
			RegisterPerHourIP:   cfg.RateLimit.RegisterPerHourIP,
			RefreshPerMinuteIP:  cfg.RateLimit.RefreshPerMinuteIP,
		},
		cfg.JWT.RefreshTTL,
		log,
	)
	authHandler := auth.NewHandler(authService, cfg.RateLimit.TrustedProxyHops)
	authMiddleware := auth.Authenticate(authService)

	llm, err := ai.NewLLM(cfg.AI)
	if err != nil {
		return nil, err
	}
	embeddings, err := ai.NewEmbedding(context.Background(), cfg.AI)
	if err != nil {
		return nil, err
	}

	userService := user.NewService(user.NewRepository(db.Gorm))
	userHandler := user.NewHandler(userService)

	queue := job.NewQueue(db.Gorm)
	accountService := account.NewService(
		account.NewRepository(db.Gorm),
		passwords,
		queue,
		limiter,
		account.Limits{PerHour: cfg.RateLimit.AccountDeletePerHour},
		log,
	)
	accountHandler := account.NewHandler(accountService)

	journalRepo := journal.NewRepository(db.Gorm)
	journalService := journal.NewService(
		journalRepo,
		queue,
		limiter,
		journal.Limits{PerMinute: cfg.RateLimit.JournalPerMinute},
	)
	journalHandler := journal.NewHandler(journalService)

	retrieval := journal.NewRetrievalService(
		journalRepo,
		embeddings,
		cfg.AI.EmbeddingModelName(),
		cfg.AI.EmbeddingDimension,
		log,
	)
	memoryRepo := memory.NewRepository(db.Gorm)
	memoryService := memory.NewService(memoryRepo, log)
	memoryHandler := memory.NewHandler(memoryService)

	insightRepo := insight.NewRepository(db.Gorm)
	insightService := insight.NewService(
		insightRepo,
		queue,
		limiter,
		usageService,
		insight.Limits{
			PerHour: cfg.RateLimit.InsightPerHour,
			PerDay:  cfg.RateLimit.InsightPerDay,
		},
		log,
	)
	insightHandler := insight.NewHandler(insightService)
	scheduleService := insight.NewScheduleService(insightRepo, queue, usageService, log)
	scheduleHandler := insight.NewScheduleHandler(scheduleService)

	conversationService := conversation.NewService(
		conversation.NewRepository(db.Gorm),
		retrieval,
		memoryService,
		queue,
		safetyService,
		usageService,
		cfg.AI.Provider,
		llm,
		limiter,
		conversation.Limits{CompanionPerMinute: cfg.RateLimit.CompanionPerMinute},
		cfg.AI.Model,
		log,
	)
	conversationHandler := conversation.NewHandler(conversationService)

	return httpx.NewRouter(httpx.RouterOptions{
		Env:              cfg.App.Env,
		Log:              log,
		Readiness:        readiness,
		ReadinessTimeout: cfg.Health.Timeout,
		MaxBodyBytes:     cfg.HTTP.MaxBodyBytes,
		Register: func(api *gin.RouterGroup) {
			authGroup := api.Group("/auth")
			authGroup.POST("/register", authHandler.Register)
			authGroup.POST("/login", authHandler.Login)
			authGroup.POST("/refresh", authHandler.Refresh)

			protected := api.Group("")
			protected.Use(authMiddleware)
			protected.POST("/auth/logout", authHandler.Logout)
			protected.GET("/me", userHandler.GetMe)
			protected.PATCH("/me", userHandler.UpdateMe)
			protected.DELETE("/me", accountHandler.DeleteMe)

			protected.GET("/journals", journalHandler.List)
			protected.POST("/journals", journalHandler.Create)
			protected.GET("/journal-changes", journalHandler.Changes)
			protected.GET("/journals/:journalId", journalHandler.Get)
			protected.PATCH("/journals/:journalId", journalHandler.Update)
			protected.DELETE("/journals/:journalId", journalHandler.Delete)

			protected.GET("/conversations", conversationHandler.List)
			protected.POST("/conversations", conversationHandler.Create)
			protected.GET("/conversations/:conversationId", conversationHandler.Get)
			protected.DELETE("/conversations/:conversationId", conversationHandler.Delete)
			protected.GET("/conversations/:conversationId/messages", conversationHandler.ListMessages)
			protected.POST("/conversations/:conversationId/messages", conversationHandler.SendMessage)

			protected.GET("/memories", memoryHandler.List)
			protected.GET("/memories/:memoryId", memoryHandler.Get)
			protected.PATCH("/memories/:memoryId", memoryHandler.Update)
			protected.DELETE("/memories/:memoryId", memoryHandler.Delete)
			protected.POST("/memories/:memoryId/feedback", memoryHandler.Feedback)
			protected.GET("/memory-consent", memoryHandler.GetConsent)
			protected.PUT("/memory-consent", memoryHandler.PutConsent)

			protected.GET("/insights", insightHandler.List)
			protected.POST("/insights", insightHandler.Request)
			protected.GET("/insight-jobs/:jobId", insightHandler.JobStatus)
			protected.GET("/insight-schedule", scheduleHandler.Get)
			protected.PUT("/insight-schedule", scheduleHandler.Put)
			protected.DELETE("/insight-schedule", scheduleHandler.Delete)
			protected.GET("/insights/:insightId", insightHandler.Get)
			protected.DELETE("/insights/:insightId", insightHandler.Delete)
			protected.POST("/insights/:insightId/feedback", insightHandler.Feedback)
		},
	}), nil
}

// New wires dependencies and prepares the HTTP server. Connectivity and
// configuration failures abort startup.
func New(ctx context.Context, cfg *config.Config, log *slog.Logger) (*App, error) {
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return nil, err
	}

	expectedVersion, err := migrations.LatestVersion()
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("read embedded migration version: %w", err)
	}

	versionCtx, cancel := context.WithTimeout(ctx, cfg.Database.PingTimeout)
	appliedVersion, dirty, versionErr := database.MigrationVersion(versionCtx, db.SQL)
	cancel()

	switch {
	case versionErr != nil:
		log.Warn("could not read applied migration version",
			slog.String("error", versionErr.Error()),
		)
	case dirty:
		log.Warn("database migration state is dirty",
			slog.Uint64("version", uint64(appliedVersion)),
		)
	case appliedVersion < expectedVersion:
		log.Warn("database schema is behind the application",
			slog.Uint64("applied_version", uint64(appliedVersion)),
			slog.Uint64("expected_version", uint64(expectedVersion)),
		)
	default:
		log.Info("database connection established",
			slog.Uint64("migration_version", uint64(appliedVersion)),
		)
	}

	router, err := BuildRouter(cfg, log, db)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	server := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.App.Port),
		Handler:           router,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
		MaxHeaderBytes:    1 << 20,
	}

	log.Info("application initialized",
		slog.String("env", cfg.App.Env),
		slog.Int("port", cfg.App.Port),
	)

	return &App{cfg: cfg, log: log, db: db, server: server}, nil
}

// Run starts the HTTP server and blocks until the context is cancelled or the
// server fails. Shutdown is bounded by HTTP_SHUTDOWN_TIMEOUT.
func (a *App) Run(ctx context.Context) error {
	listenErr := make(chan error, 1)

	go func() {
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			listenErr <- err
			return
		}
		listenErr <- nil
	}()

	select {
	case err := <-listenErr:
		a.closeDatabase()
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	case <-ctx.Done():
	}

	a.log.Info("shutdown requested")
	a.server.SetKeepAlivesEnabled(false)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.HTTP.ShutdownTimeout)
	defer cancel()

	shutdownErr := a.server.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		a.log.Error("graceful shutdown failed", slog.String("error", shutdownErr.Error()))
		_ = a.server.Close()
	}

	err := <-listenErr
	a.closeDatabase()

	if shutdownErr != nil {
		return fmt.Errorf("shutdown http server: %w", shutdownErr)
	}
	if err != nil {
		return fmt.Errorf("http server: %w", err)
	}

	a.log.Info("shutdown complete")
	return nil
}

func (a *App) closeDatabase() {
	if err := a.db.Close(); err != nil {
		a.log.Warn("close database pool", slog.String("error", err.Error()))
	}
}
