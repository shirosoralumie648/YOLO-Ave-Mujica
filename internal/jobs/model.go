package jobs

import "time"

const (
	StatusQueued              = "queued"
	StatusRunning             = "running"
	StatusSucceeded           = "succeeded"
	StatusSucceededWithErrors = "succeeded_with_errors"
	StatusFailed              = "failed"
	StatusCanceled            = "canceled"
	StatusRetryWaiting        = "retry_waiting"
)

type Job struct {
	ID                   int64
	ProjectID            int64
	JobType              string
	Status               string
	RequiredResourceType string
	IdempotencyKey       string
	Payload              map[string]any
	TotalItems           int
	SucceededItems       int
	FailedItems          int
	CreatedAt            time.Time
	StartedAt            *time.Time
	FinishedAt           *time.Time
}
