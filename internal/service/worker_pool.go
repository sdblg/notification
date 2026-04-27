package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sdblg/notification/pkg/models"
)

var ErrQueueFull = errors.New("notification queue is full")

type NotificationJob struct {
	TraceID   string
	RequestID string
	Message   models.EmailMessage
}

type WorkerPool struct {
	workers int
	queue   chan NotificationJob
	sender  *FailoverEmailService
	logger  *slog.Logger
	wg      sync.WaitGroup
}

func NewWorkerPool(workers, queueSize int, sender *FailoverEmailService, logger *slog.Logger) (*WorkerPool, error) {
	if workers <= 0 {
		return nil, fmt.Errorf("workers must be greater than 0")
	}
	if queueSize <= 0 {
		return nil, fmt.Errorf("queueSize must be greater than 0")
	}
	if sender == nil {
		return nil, fmt.Errorf("sender is required")
	}

	return &WorkerPool{
		workers: workers,
		queue:   make(chan NotificationJob, queueSize),
		sender:  sender,
		logger:  logger,
	}, nil
}

func (p *WorkerPool) Start(ctx context.Context) {
	for i := 0; i < p.workers; i++ {
		workerID := i + 1
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.workerLoop(ctx, workerID)
		}()
	}
}

func (p *WorkerPool) Stop() {
	close(p.queue)
	p.wg.Wait()
}

func (p *WorkerPool) Enqueue(ctx context.Context, msg models.EmailMessage) (string, error) {
	reqID := uuid.NewString()
	job := NotificationJob{
		TraceID:   TraceIDFromContext(ctx),
		RequestID: reqID,
		Message:   msg,
	}

	select {
	case p.queue <- job:
		return reqID, nil
	case <-ctx.Done():
		return "", ctx.Err()
	default:
		return "", ErrQueueFull
	}
}

func (p *WorkerPool) workerLoop(ctx context.Context, workerID int) {
	logger := p.logger
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("component", "worker_pool", "worker_id", workerID)
	logger.Info("worker started")
	defer logger.Info("worker stopped")

	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-p.queue:
			if !ok {
				return
			}

			sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			err := p.sender.Send(sendCtx, job.Message)
			cancel()

			jobLog := logger
			if job.TraceID != "" {
				jobLog = jobLog.With("trace_id", job.TraceID)
			}
			if err != nil {
				jobLog.Error("send failed", "request_id", job.RequestID, "to", job.Message.To, "error", err)
				continue
			}
			jobLog.Info("send succeeded", "request_id", job.RequestID, "to", job.Message.To)
		}
	}
}
