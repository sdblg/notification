package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sdblg/notification/internal/provider"
	"github.com/sdblg/notification/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg := loadConfig()
	if strings.TrimSpace(cfg.APIKey) == "" {
		logger.Error("missing required configuration", "key", "API_KEY")
		os.Exit(1)
	}

	providers := make([]provider.EmailProvider, 0, 2)

	if p, err := provider.NewResendProviderFromEnv(); err == nil {
		providers = append(providers, p)
	} else {
		logger.Warn("resend disabled", "error", err)
	}

	if p, err := provider.NewBrevoProviderFromEnv(); err == nil {
		providers = append(providers, p)
	} else {
		logger.Warn("brevo disabled", "error", err)
	}

	failoverSvc, err := service.NewFailoverEmailService(providers...)
	if err != nil {
		logger.Error("failed to initialize providers", "error", err)
		os.Exit(1)
	}

	workerPool, err := service.NewWorkerPool(cfg.Workers, cfg.QueueSize, failoverSvc, logger)
	if err != nil {
		logger.Error("failed to initialize worker pool", "error", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	workerPool.Start(ctx)

	mux := http.NewServeMux()
	notifyHandler := service.NewNotifyHandler(workerPool, cfg.MailFrom, logger)
	mux.Handle("/v1/notify", service.TraceIDMiddleware(
		service.APIKeyAuthMiddleware(cfg.APIKey, notifyHandler, logger)))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("notification api listening", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown error", "error", err)
	}

	workerPool.Stop()
	logger.Info("notification service stopped")
}

type Config struct {
	Port      string
	Workers   int
	QueueSize int
	MailFrom  string
	APIKey    string
}

func loadConfig() Config {
	return Config{
		Port:      getEnv("APP_PORT", "8080"),
		Workers:   getEnvAsInt("WORKER_COUNT", 4),
		QueueSize: getEnvAsInt("QUEUE_SIZE", 100),
		MailFrom:  getEnv("MAIL_FROM", "no-reply@mongols.app"),
		APIKey:    getEnv("API_KEY", ""),
	}
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getEnvAsInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}
