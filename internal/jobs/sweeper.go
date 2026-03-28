package jobs

import (
	"context"
	"time"
)

type Sweeper struct {
	repo       Repository
	dispatcher Publisher
	maxRetries int
}

func NewSweeper(repo Repository, dispatcher Publisher, maxRetries int) *Sweeper {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return &Sweeper{
		repo:       repo,
		dispatcher: dispatcher,
		maxRetries: maxRetries,
	}
}

func (s *Sweeper) Tick(now time.Time) error {
	expired := s.repo.ListExpiredRunning(now)
	for _, job := range expired {
		workerID := job.WorkerID
		if job.RetryCount >= s.maxRetries {
			if err := s.repo.MarkFailed(job.ID, "lease_timeout", "retry exhausted"); err != nil {
				return err
			}
			if _, err := s.repo.AppendEvent(job.ID, nil, "error", "lease_timeout", "job failed after lease expiry", map[string]any{"worker_id": workerID}); err != nil {
				return err
			}
			continue
		}
		if err := s.repo.MarkRetryWaiting(job.ID); err != nil {
			return err
		}
		if err := s.repo.IncrementRetryCount(job.ID); err != nil {
			return err
		}
		if err := s.repo.UpdateStatus(job.ID, StatusQueued); err != nil {
			return err
		}
		if _, err := s.repo.AppendEvent(job.ID, nil, "warn", "lease_recovered", "lease expired; job requeued", map[string]any{"worker_id": workerID}); err != nil {
			return err
		}
		if s.dispatcher != nil {
			if err := s.dispatcher.Publish(context.Background(), laneFor(job.RequiredResourceType), buildDispatchPayload(job)); err != nil {
				return err
			}
		}
	}
	return nil
}
