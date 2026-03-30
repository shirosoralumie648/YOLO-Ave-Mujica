package jobs

import "time"

const (
	// StatusQueued means the job has been accepted and is waiting for a worker.
	StatusQueued = "queued"
	// StatusRunning means a worker currently holds the lease and is executing the job.
	StatusRunning = "running"
	// StatusSucceeded means all tracked work completed without item-level failures.
	StatusSucceeded = "succeeded"
	// StatusSucceededWithErrors means some items failed but the overall job still produced useful output.
	StatusSucceededWithErrors = "succeeded_with_errors"
	// StatusFailed means the job terminated without a successful result.
	StatusFailed = "failed"
	// StatusCanceled means execution stopped due to an external cancellation request.
	StatusCanceled = "canceled"
	// StatusRetryWaiting means the job is paused until it becomes eligible for retry.
	StatusRetryWaiting = "retry_waiting"
)

// Job is the canonical persisted runtime record for asynchronous work.
// It stores routing information, execution counters, lease state, and terminal errors.
type Job struct {
	ID                   int64
	ProjectID            int64
	DatasetID            int64
	SnapshotID           int64
	JobType              string
	Status               string
	RequiredResourceType string
	RequiredCapabilities []string
	IdempotencyKey       string
	WorkerID             string
	Payload              map[string]any
	TotalItems           int
	SucceededItems       int
	FailedItems          int
	CreatedAt            time.Time
	StartedAt            *time.Time
	FinishedAt           *time.Time
	LeaseUntil           *time.Time
	RetryCount           int
	ErrorCode            string
	ErrorMsg             string
}

type CreateJobInput struct {
	ProjectID            int64
	DatasetID            int64
	SnapshotID           int64
	JobType              string
	RequiredResourceType string
	RequiredCapabilities []string
	IdempotencyKey       string
	Payload              map[string]any
}
