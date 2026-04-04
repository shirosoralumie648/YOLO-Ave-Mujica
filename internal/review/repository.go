package review

import (
	"fmt"
	"sync"
	"time"
)

type Repository interface {
	ListPending() ([]Candidate, error)
	Accept(candidateID int64, reviewer string) error
	Reject(candidateID int64, reviewer, reasonCode string) error
	ListPublishableCandidates(projectID int64) ([]PublishableCandidate, error)
	PersistCandidates(jobID int64, items []PersistCandidateInput) ([]Candidate, error)
}

type PublishableCandidate struct {
	ID           int64          `json:"id"`
	ProjectID    int64          `json:"project_id"`
	DatasetID    int64          `json:"dataset_id"`
	SnapshotID   int64          `json:"snapshot_id"`
	ItemID       int64          `json:"item_id"`
	TaskID       int64          `json:"task_id"`
	ReviewStatus string         `json:"review_status"`
	RiskLevel    string         `json:"risk_level"`
	SourceModel  string         `json:"source_model"`
	AcceptedAt   time.Time      `json:"accepted_at"`
	Summary      map[string]any `json:"summary"`
}

type InMemoryRepository struct {
	mu          sync.Mutex
	nextID      int64
	candidates  map[int64]Candidate
	annotations []Annotation
	audits      []AuditEvent
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		nextID:      1,
		candidates:  make(map[int64]Candidate),
		annotations: []Annotation{},
		audits:      []AuditEvent{},
	}
}

func (r *InMemoryRepository) SeedCandidate(c Candidate) {
	r.mu.Lock()
	defer r.mu.Unlock()
	status := c.Status
	if status == "" {
		status = c.ReviewStatus
	}
	if status == "" {
		status = CandidateStatusQueuedForReview
	}
	c.Status = normalizeCandidateStatus(status)
	c.ReviewStatus = c.Status
	r.candidates[c.ID] = normalizeCandidate(c)
}

func (r *InMemoryRepository) ListPending() ([]Candidate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]Candidate, 0, len(r.candidates))
	for _, c := range r.candidates {
		status := c.Status
		if status == "" {
			status = c.ReviewStatus
		}
		if isQueuedCandidateStatus(status) {
			out = append(out, normalizeCandidate(c))
		}
	}
	return out, nil
}

func (r *InMemoryRepository) Accept(candidateID int64, reviewer string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.candidates[candidateID]
	if !ok {
		return fmt.Errorf("candidate %d not found", candidateID)
	}
	if !isQueuedCandidateStatus(c.Status) && !isQueuedCandidateStatus(c.ReviewStatus) {
		status := c.Status
		if status == "" {
			status = c.ReviewStatus
		}
		return fmt.Errorf("candidate is %s", normalizeCandidateStatus(status))
	}

	now := time.Now().UTC()
	c.Status = "accepted"
	c.ReviewStatus = "accepted"
	c.ReviewerID = reviewer
	c.ReviewedAt = now
	r.candidates[candidateID] = normalizeCandidate(c)
	r.annotations = append(r.annotations, Annotation{CandidateID: candidateID, ReviewerID: reviewer, CreatedAt: now})
	r.audits = append(r.audits, AuditEvent{Actor: reviewer, Action: "review.accept", ResourceType: "annotation_candidate", ResourceID: fmt.Sprintf("%d", candidateID), TS: now})
	return nil
}

func (r *InMemoryRepository) Reject(candidateID int64, reviewer, reasonCode string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.candidates[candidateID]
	if !ok {
		return fmt.Errorf("candidate %d not found", candidateID)
	}
	if !isQueuedCandidateStatus(c.Status) && !isQueuedCandidateStatus(c.ReviewStatus) {
		status := c.Status
		if status == "" {
			status = c.ReviewStatus
		}
		return fmt.Errorf("candidate is %s", normalizeCandidateStatus(status))
	}

	now := time.Now().UTC()
	c.Status = "rejected"
	c.ReviewStatus = "rejected"
	c.ReviewerID = reviewer
	c.ReviewedAt = now
	r.candidates[candidateID] = normalizeCandidate(c)
	r.audits = append(r.audits, AuditEvent{
		Actor:        reviewer,
		Action:       "review.reject",
		ResourceType: "annotation_candidate",
		ResourceID:   fmt.Sprintf("%d", candidateID),
		Detail: map[string]any{
			"reason_code": reasonCode,
		},
		TS: now,
	})
	return nil
}

func (r *InMemoryRepository) ListPublishableCandidates(projectID int64) ([]PublishableCandidate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	items := make([]PublishableCandidate, 0, len(r.candidates))
	for _, candidate := range r.candidates {
		if normalizeCandidateStatus(candidate.ReviewStatus) != "accepted" && normalizeCandidateStatus(candidate.Status) != "accepted" {
			continue
		}
		candidate = normalizeCandidate(candidate)
		items = append(items, PublishableCandidate{
			ID:           candidate.ID,
			ProjectID:    projectID,
			DatasetID:    candidate.DatasetID,
			SnapshotID:   candidate.SnapshotID,
			ItemID:       candidate.ItemID,
			ReviewStatus: candidate.ReviewStatus,
			RiskLevel:    "normal",
			SourceModel:  "",
			AcceptedAt:   candidate.ReviewedAt,
			Summary:      map[string]any{},
		})
	}
	return items, nil
}

func (r *InMemoryRepository) PersistCandidates(jobID int64, items []PersistCandidateInput) ([]Candidate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	out := make([]Candidate, 0, len(items))
	for _, item := range items {
		id := r.nextID
		r.nextID++

		candidate := normalizeCandidate(Candidate{
			ID:           id,
			DatasetID:    item.DatasetID,
			SnapshotID:   item.SnapshotID,
			ItemID:       item.ItemID,
			ObjectKey:    item.ObjectKey,
			CategoryID:   item.CategoryID,
			BBox:         item.BBox,
			Status:       CandidateStatusQueuedForReview,
			ReviewStatus: CandidateStatusQueuedForReview,
			Source: CandidateSource{
				JobID:      &jobID,
				Confidence: item.Confidence,
				ModelName:  item.ModelName,
				IsPseudo:   true,
				CreatedAt:  &now,
			},
		})
		r.candidates[candidate.ID] = candidate
		out = append(out, candidate)
	}
	return out, nil
}

func (r *InMemoryRepository) AnnotationCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.annotations)
}

func (r *InMemoryRepository) GetCandidate(id int64) (Candidate, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.candidates[id]
	return normalizeCandidate(c), ok
}

var _ Repository = (*InMemoryRepository)(nil)
