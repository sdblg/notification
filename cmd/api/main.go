package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sod/notification/internal/provider"
	"github.com/sod/notification/internal/service"
)

func main() {
	cfg := loadConfig()

	providers := make([]provider.EmailProvider, 0, 2)

	if p, err := provider.NewResendProviderFromEnv(); err == nil {
		providers = append(providers, p)
	} else {
		log.Printf("resend disabled: %v", err)
	}

	if p, err := provider.NewBrevoProviderFromEnv(); err == nil {
		providers = append(providers, p)
	} else {
		log.Printf("brevo disabled: %v", err)
	}

	failoverSvc, err := service.NewFailoverEmailService(providers...)
	if err != nil {
		log.Fatalf("failed to initialize providers: %v", err)
	}

	workerPool, err := service.NewWorkerPool(cfg.Workers, cfg.QueueSize, failoverSvc)
	if err != nil {
		log.Fatalf("failed to initialize worker pool: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	workerPool.Start(ctx)

	mux := http.NewServeMux()
	mux.Handle("/v1/notify", service.NewNotifyHandler(workerPool, cfg.MailFrom))
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
		log.Printf("notification api listening on :%s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutdown signal received")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown error: %v", err)
	}

	workerPool.Stop()
	log.Println("notification service stopped")
}

type Config struct {
	Port      string
	Workers   int
	QueueSize int
	MailFrom  string
}

func loadConfig() Config {
	return Config{
		Port:      getEnv("APP_PORT", "8080"),
		Workers:   getEnvAsInt("WORKER_COUNT", 4),
		QueueSize: getEnvAsInt("QUEUE_SIZE", 100),
		MailFrom:  getEnv("MAIL_FROM", "no-reply@mongols.app"),
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
