package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sod/notification/internal/provider"
)

var ErrQueueFull = errors.New("notification queue is full")

type NotificationJob struct {
	RequestID string
	Message   provider.EmailMessage
}

type WorkerPool struct {
	workers int
	queue   chan NotificationJob
	sender  *FailoverEmailService
	wg      sync.WaitGroup
}

func NewWorkerPool(workers, queueSize int, sender *FailoverEmailService) (*WorkerPool, error) {
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

func (p *WorkerPool) Enqueue(ctx context.Context, msg provider.EmailMessage) (string, error) {
	reqID := uuid.NewString()
	job := NotificationJob{
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
	log.Printf("worker %d started", workerID)
	defer log.Printf("worker %d stopped", workerID)

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

			if err != nil {
				log.Printf("worker=%d request_id=%s status=failed err=%v", workerID, job.RequestID, err)
				continue
			}
			log.Printf("worker=%d request_id=%s status=sent to=%s", workerID, job.RequestID, job.Message.To)
		}
	}
}
