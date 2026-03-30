package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type Event struct {
	ID         int64          `json:"id"`
	JobID      int64          `json:"job_id"`
	ItemID     *int64         `json:"item_id,omitempty"`
	EventLevel string         `json:"event_level"`
	EventType  string         `json:"event_type"`
	Message    string         `json:"message"`
	Detail     map[string]any `json:"detail_json,omitempty"`
	TS         time.Time      `json:"ts"`
}

type Service struct {
	repo       Repository
	dispatcher Publisher
}

func NewService(repo Repository) *Service {
	return NewServiceWithPublisher(repo, nil)
}

func NewServiceWithPublisher(repo Repository, dispatcher Publisher) *Service {
	if repo == nil {
		repo = NewInMemoryRepository()
	}
	return &Service{repo: repo, dispatcher: dispatcher}
}

func (s *Service) CreateJob(in CreateJobInput) (*Job, error) {
	if in.ProjectID <= 0 {
		in.ProjectID = 1
	}
	if in.IdempotencyKey == "" {
		return nil, errors.New("idempotency_key is required")
	}
	if in.RequiredResourceType == "" {
		in.RequiredResourceType = "cpu"
	}

	job, created, err := s.repo.CreateOrGet(in)
	if err != nil {
		return nil, err
	}
	if created {
		if _, err := s.repo.AppendEvent(job.ID, nil, "info", "queued", "job queued", map[string]any{"job_type": in.JobType}); err != nil {
			return nil, err
		}
		if s.dispatcher != nil {
			if err := s.dispatcher.Publish(context.Background(), laneFor(job.RequiredResourceType), buildDispatchPayload(job)); err != nil {
				return nil, err
			}
		}
	}
	return job, nil
}

func (s *Service) GetJob(id int64) (*Job, bool) {
	return s.repo.Get(id)
}

func (s *Service) AppendEvent(jobID int64, itemID *int64, level, typ, message string, detail map[string]any) Event {
	ev, _ := s.repo.AppendEvent(jobID, itemID, level, typ, message, detail)
	return ev
}

func (s *Service) ListEvents(jobID int64) []Event {
	out, _ := s.repo.ListEvents(jobID)
	return out
}

func (s *Service) ReportHeartbeat(jobID int64, workerID string, leaseSeconds int) error {
	if workerID == "" {
		return errors.New("worker_id is required")
	}
	if leaseSeconds <= 0 {
		leaseSeconds = 30
	}

	leaseUntil := time.Now().UTC().Add(time.Duration(leaseSeconds) * time.Second)
	job, ok := s.repo.Get(jobID)
	if !ok {
		return fmt.Errorf("job %d not found", jobID)
	}

	if job.Status == StatusRunning {
		if err := s.repo.TouchLease(jobID, workerID, leaseUntil); err != nil {
			return err
		}
	} else {
		if _, err := s.repo.Claim(jobID, workerID, leaseUntil); err != nil {
			return err
		}
	}

	_, err := s.repo.AppendEvent(jobID, nil, "info", "heartbeat", "worker heartbeat", map[string]any{
		"worker_id":     workerID,
		"lease_seconds": leaseSeconds,
	})
	return err
}

func (s *Service) ReportProgress(jobID int64, workerID string, total, succeeded, failed int) error {
	if workerID == "" {
		return errors.New("worker_id is required")
	}
	if total < 0 || succeeded < 0 || failed < 0 {
		return errors.New("progress counters must be >= 0")
	}
	if err := s.repo.UpdateProgress(jobID, workerID, total, succeeded, failed); err != nil {
		return err
	}
	_, err := s.repo.AppendEvent(jobID, nil, "info", "progress", "worker progress", map[string]any{
		"worker_id":       workerID,
		"total_items":     total,
		"succeeded_items": succeeded,
		"failed_items":    failed,
	})
	return err
}

func (s *Service) ReportItemError(jobID, itemID int64, message string, detail map[string]any) error {
	if itemID <= 0 {
		return errors.New("item_id must be > 0")
	}
	if message == "" {
		return errors.New("message is required")
	}
	_, err := s.repo.AppendEvent(jobID, &itemID, "error", "item_failed", message, detail)
	return err
}

func (s *Service) ReportTerminal(jobID int64, workerID, status string, total, succeeded, failed int) error {
	if workerID == "" {
		return errors.New("worker_id is required")
	}
	switch status {
	case StatusSucceeded, StatusSucceededWithErrors, StatusFailed, StatusCanceled:
	default:
		return fmt.Errorf("unsupported terminal status %q", status)
	}
	if total < 0 || succeeded < 0 || failed < 0 {
		return errors.New("terminal counters must be >= 0")
	}
	if err := s.repo.Complete(jobID, workerID, status, total, succeeded, failed); err != nil {
		return err
	}
	_, err := s.repo.AppendEvent(jobID, nil, "info", "terminal", "job completed", map[string]any{
		"worker_id":       workerID,
		"status":          status,
		"total_items":     total,
		"succeeded_items": succeeded,
		"failed_items":    failed,
	})
	return err
}
