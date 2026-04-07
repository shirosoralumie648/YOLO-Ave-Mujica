package jobs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"yolo-ave-mujica/internal/observability"
)

type Sweeper struct {
	repo        Repository
	dispatcher  Publisher
	maxRetries  int
	metrics     *observability.Metrics
	backoffBase time.Duration
	backoffMax  time.Duration
}

func NewSweeper(repo Repository, dispatcher Publisher, maxRetries int) *Sweeper {
	return NewSweeperWithMetrics(repo, dispatcher, maxRetries, nil)
}

func NewSweeperWithMetrics(repo Repository, dispatcher Publisher, maxRetries int, metrics *observability.Metrics) *Sweeper {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return &Sweeper{
		repo:        repo,
		dispatcher:  dispatcher,
		maxRetries:  maxRetries,
		metrics:     metrics,
		backoffBase: 5 * time.Second,
		backoffMax:  time.Minute,
	}
}

func (s *Sweeper) WithRetryBackoff(base, max time.Duration) *Sweeper {
	if base <= 0 {
		base = 5 * time.Second
	}
	if max < base {
		max = base
	}
	s.backoffBase = base
	s.backoffMax = max
	return s
}

func (s *Sweeper) Tick(now time.Time) error {
	for _, job := range s.repo.ListRetryReady(now) {
		retryAt := job.LeaseUntil
		if err := s.repo.UpdateStatus(job.ID, StatusQueued); err != nil {
			return err
		}
		if _, err := s.repo.AppendEvent(job.ID, nil, "info", "retry_requeued", "retry backoff elapsed; job requeued", map[string]any{
			"retry_count":   job.RetryCount,
			"resource_lane": laneFor(job.RequiredResourceType),
			"retry_at":      retryAt,
		}); err != nil {
			return err
		}
		if s.dispatcher != nil {
			if err := s.dispatcher.Publish(context.Background(), laneFor(job.RequiredResourceType), buildDispatchPayload(job)); err != nil {
				return err
			}
		}
	}

	expired := s.repo.ListExpiredRunning(now)
	for _, job := range expired {
		workerID := job.WorkerID
		classification := classifyLeaseRetry(job)
		if classification == "fatal" {
			if err := s.repo.MarkFailed(job.ID, "lease_timeout", "fatal retry classification"); err != nil {
				return err
			}
			if _, err := s.repo.AppendEvent(job.ID, nil, "error", "lease_timeout", "job failed after lease expiry", map[string]any{
				"worker_id":            workerID,
				"retry_classification": classification,
			}); err != nil {
				return err
			}
			continue
		}

		nextRetryCount := job.RetryCount + 1
		if job.RetryCount >= s.maxRetries {
			if err := s.repo.MarkFailed(job.ID, "lease_timeout", "retry exhausted"); err != nil {
				return err
			}
			if _, err := s.repo.AppendEvent(job.ID, nil, "error", "lease_timeout", "job failed after lease expiry", map[string]any{
				"worker_id":            workerID,
				"retry_classification": classification,
				"retry_count":          job.RetryCount,
			}); err != nil {
				return err
			}
			continue
		}

		backoff := s.retryBackoffDuration(nextRetryCount)
		retryAt := now.UTC().Add(backoff)
		if err := s.repo.MarkRetryWaiting(job.ID, retryAt); err != nil {
			return err
		}
		if err := s.repo.IncrementRetryCount(job.ID); err != nil {
			return err
		}
		if _, err := s.repo.AppendEvent(job.ID, nil, "warn", "lease_recovered", "lease expired; job scheduled for retry", map[string]any{
			"worker_id":             workerID,
			"retry_classification":  classification,
			"retry_count":           nextRetryCount,
			"retry_backoff_seconds": backoff.Seconds(),
			"retry_at":              retryAt,
		}); err != nil {
			return err
		}
		if s.metrics != nil {
			s.metrics.IncLeaseRecovery(job.JobType)
		}
	}
	return nil
}

func classifyLeaseRetry(job *Job) string {
	if job == nil || job.Payload == nil {
		return "transient"
	}
	rawPolicy, ok := job.Payload["retry_policy"]
	if !ok {
		return "transient"
	}
	policy, ok := rawPolicy.(map[string]any)
	if !ok {
		return "transient"
	}
	classification := strings.TrimSpace(strings.ToLower(fmt.Sprint(policy["classification"])))
	switch classification {
	case "fatal":
		return "fatal"
	case "transient":
		return "transient"
	default:
		return "transient"
	}
}

func (s *Sweeper) retryBackoffDuration(retryCount int) time.Duration {
	if retryCount <= 1 {
		return s.backoffBase
	}
	delay := s.backoffBase
	for attempt := 1; attempt < retryCount; attempt++ {
		if delay >= s.backoffMax {
			return s.backoffMax
		}
		if delay > s.backoffMax/2 {
			return s.backoffMax
		}
		delay *= 2
	}
	if delay > s.backoffMax {
		return s.backoffMax
	}
	return delay
}
