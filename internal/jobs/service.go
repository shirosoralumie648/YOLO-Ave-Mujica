package jobs

import (
	"context"
	"errors"
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

func (s *Service) CreateJob(projectID int64, jobType, requiredResourceType, idempotencyKey string, payload map[string]any) (*Job, error) {
	if projectID <= 0 {
		projectID = 1
	}
	if idempotencyKey == "" {
		return nil, errors.New("idempotency_key is required")
	}
	if requiredResourceType == "" {
		requiredResourceType = "cpu"
	}

	job, created, err := s.repo.CreateOrGet(projectID, jobType, requiredResourceType, idempotencyKey, payload)
	if err != nil {
		return nil, err
	}
	if created {
		if _, err := s.repo.AppendEvent(job.ID, nil, "info", "queued", "job queued", map[string]any{"job_type": jobType}); err != nil {
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
