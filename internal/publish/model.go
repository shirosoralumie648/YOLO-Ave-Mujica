package publish

import "time"

const (
	SourceSuggested = "suggested"
	SourceManual    = "manual"

	StatusDraft                 = "draft"
	StatusReviewPending         = "review_pending"
	StatusReviewApproved        = "review_approved"
	StatusOwnerPending          = "owner_pending"
	StatusOwnerChangesRequested = "owner_changes_requested"
	StatusRejected              = "rejected"
	StatusPublished             = "published"
	StatusSuperseded            = "superseded"

	FeedbackScopeBatch = "batch"
	FeedbackScopeItem  = "item"

	FeedbackStageReview = "review"
	FeedbackStageOwner  = "owner"

	FeedbackActionReject  = "reject"
	FeedbackActionRework  = "rework"
	FeedbackActionComment = "comment"

	ReviewDecisionApprove = "approve"
	ReviewDecisionReject  = "reject"
	ReviewDecisionRework  = "rework"

	OwnerDecisionApprove = "approve"
	OwnerDecisionReject  = "reject"
	OwnerDecisionRework  = "rework"
)

type Batch struct {
	ID               int64          `json:"id"`
	ProjectID        int64          `json:"project_id"`
	SnapshotID       int64          `json:"snapshot_id"`
	Source           string         `json:"source"`
	Status           string         `json:"status"`
	RuleSummary      map[string]any `json:"rule_summary"`
	OwnerEditVersion int            `json:"owner_edit_version"`
	ReviewApprovedAt *time.Time     `json:"review_approved_at,omitempty"`
	ReviewApprovedBy string         `json:"review_approved_by,omitempty"`
	OwnerDecidedAt   *time.Time     `json:"owner_decided_at,omitempty"`
	OwnerDecidedBy   string         `json:"owner_decided_by,omitempty"`
	Items            []BatchItem    `json:"items,omitempty"`
	Feedback         []Feedback     `json:"feedback,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

type BatchItem struct {
	ID             int64          `json:"id"`
	PublishBatchID int64          `json:"publish_batch_id"`
	CandidateID    int64          `json:"candidate_id"`
	TaskID         int64          `json:"task_id"`
	DatasetID      int64          `json:"dataset_id"`
	SnapshotID     int64          `json:"snapshot_id"`
	ItemPayload    map[string]any `json:"item_payload"`
	Position       int            `json:"position"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type Feedback struct {
	ID                 int64     `json:"id"`
	PublishBatchID     int64     `json:"publish_batch_id"`
	PublishBatchItemID int64     `json:"publish_batch_item_id,omitempty"`
	Scope              string    `json:"scope"`
	Stage              string    `json:"stage"`
	Action             string    `json:"action"`
	ReasonCode         string    `json:"reason_code"`
	Severity           string    `json:"severity"`
	InfluenceWeight    float64   `json:"influence_weight"`
	Comment            string    `json:"comment"`
	CreatedBy          string    `json:"created_by"`
	CreatedAt          time.Time `json:"created_at"`
}

type Record struct {
	ID              int64          `json:"id"`
	ProjectID       int64          `json:"project_id"`
	SnapshotID      int64          `json:"snapshot_id"`
	PublishBatchID  int64          `json:"publish_batch_id"`
	Status          string         `json:"status"`
	Summary         map[string]any `json:"summary"`
	ApprovedByOwner string         `json:"approved_by_owner"`
	ApprovedAt      time.Time      `json:"approved_at"`
	CreatedAt       time.Time      `json:"created_at"`
}

type SuggestedCandidate struct {
	SnapshotID    int64                    `json:"snapshot_id"`
	SuggestionKey string                   `json:"suggestion_key"`
	Summary       map[string]any           `json:"summary"`
	Items         []SuggestedCandidateItem `json:"items"`
}

type SuggestedCandidateItem struct {
	CandidateID int64          `json:"candidate_id"`
	TaskID      int64          `json:"task_id"`
	DatasetID   int64          `json:"dataset_id"`
	ItemPayload map[string]any `json:"item_payload"`
}

type CreateBatchInput struct {
	ProjectID   int64                  `json:"project_id"`
	SnapshotID  int64                  `json:"snapshot_id"`
	Source      string                 `json:"source"`
	RuleSummary map[string]any         `json:"rule_summary"`
	Items       []CreateBatchItemInput `json:"items"`
}

type CreateBatchItemInput struct {
	CandidateID int64          `json:"candidate_id"`
	TaskID      int64          `json:"task_id"`
	DatasetID   int64          `json:"dataset_id"`
	SnapshotID  int64          `json:"snapshot_id"`
	ItemPayload map[string]any `json:"item_payload"`
}

type ReplaceBatchItemsInput struct {
	Actor string                 `json:"actor"`
	Items []CreateBatchItemInput `json:"items"`
}

type CreateFeedbackInput struct {
	Scope           string  `json:"scope"`
	Stage           string  `json:"stage"`
	Action          string  `json:"action"`
	ReasonCode      string  `json:"reason_code"`
	Severity        string  `json:"severity"`
	InfluenceWeight float64 `json:"influence_weight"`
	Comment         string  `json:"comment"`
	Actor           string  `json:"actor"`
}
