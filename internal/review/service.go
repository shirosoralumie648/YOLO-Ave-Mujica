package review

import (
	"fmt"
	"strings"
	"time"
)

const (
	CandidateStatusQueuedForReview = "queued_for_review"
	legacyCandidateStatusPending   = "pending"
)

type CandidateSource struct {
	JobID      *int64     `json:"job_id,omitempty"`
	Confidence *float64   `json:"confidence,omitempty"`
	ModelName  string     `json:"model_name,omitempty"`
	IsPseudo   bool       `json:"is_pseudo"`
	CreatedAt  *time.Time `json:"created_at,omitempty"`
}

type CandidateBBox struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

type PersistCandidateInput struct {
	DatasetID    int64
	SnapshotID   int64
	ItemID       int64
	ObjectKey    string
	CategoryID   int64
	CategoryName string
	BBox         CandidateBBox
	Confidence   *float64
	ModelName    string
	IsPseudo     bool
}

type Candidate struct {
	ID           int64           `json:"id"`
	ProjectID    int64           `json:"project_id,omitempty"`
	DatasetID    int64           `json:"dataset_id"`
	SnapshotID   int64           `json:"snapshot_id"`
	ItemID       int64           `json:"item_id"`
	ObjectKey    string          `json:"object_key,omitempty"`
	CategoryID   int64           `json:"category_id"`
	BBox         CandidateBBox   `json:"bbox"`
	Status       string          `json:"status"`
	ReviewStatus string          `json:"review_status"`
	ReviewerID   string          `json:"reviewer_id,omitempty"`
	ReviewedAt   time.Time       `json:"reviewed_at,omitempty"`
	Source       CandidateSource `json:"source"`
}

type Annotation struct {
	CandidateID int64     `json:"candidate_id"`
	ReviewerID  string    `json:"reviewer_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type AuditEvent struct {
	Actor        string         `json:"actor"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	Detail       map[string]any `json:"detail,omitempty"`
	TS           time.Time      `json:"ts"`
}

type Service struct {
	repo Repository
}

func NewService() *Service {
	return NewServiceWithRepository(nil)
}

func NewServiceWithRepository(repo Repository) *Service {
	if repo == nil {
		repo = NewInMemoryRepository()
	}
	return &Service{repo: repo}
}

func (s *Service) SeedCandidate(c Candidate) {
	if repo, ok := s.repo.(interface{ SeedCandidate(Candidate) }); ok {
		repo.SeedCandidate(c)
	}
}

func (s *Service) ListCandidates() []Candidate {
	items, err := s.repo.ListPending()
	if err != nil {
		return nil
	}
	return normalizeCandidates(items)
}

func (s *Service) AcceptCandidate(candidateID int64, reviewer string) error {
	return s.repo.Accept(candidateID, reviewer)
}

func (s *Service) RejectCandidate(candidateID int64, reviewer, reasonCode string) error {
	reasonCode = strings.TrimSpace(reasonCode)
	if reasonCode == "" {
		return fmt.Errorf("reason_code is required")
	}
	return s.repo.Reject(candidateID, reviewer, reasonCode)
}

func (s *Service) PersistCandidates(jobID int64, items []PersistCandidateInput) ([]Candidate, error) {
	if jobID <= 0 {
		return nil, fmt.Errorf("job_id is required")
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("at least one candidate is required")
	}

	normalized := make([]PersistCandidateInput, 0, len(items))
	for _, item := range items {
		if item.DatasetID <= 0 || item.SnapshotID <= 0 || item.ItemID <= 0 {
			return nil, fmt.Errorf("dataset_id, snapshot_id, and item_id are required")
		}
		if item.CategoryID <= 0 && strings.TrimSpace(item.CategoryName) == "" {
			return nil, fmt.Errorf("category_id or category_name is required")
		}
		if item.BBox.W <= 0 || item.BBox.H <= 0 {
			return nil, fmt.Errorf("bbox.w and bbox.h must be > 0")
		}
		item.CategoryName = strings.TrimSpace(item.CategoryName)
		item.ModelName = strings.TrimSpace(item.ModelName)
		item.IsPseudo = true
		normalized = append(normalized, item)
	}
	return s.repo.PersistCandidates(jobID, normalized)
}

func (s *Service) AnnotationCount() int {
	if repo, ok := s.repo.(interface{ AnnotationCount() int }); ok {
		return repo.AnnotationCount()
	}
	return 0
}

func (s *Service) GetCandidate(id int64) (Candidate, bool) {
	if repo, ok := s.repo.(interface {
		GetCandidate(id int64) (Candidate, bool)
	}); ok {
		candidate, found := repo.GetCandidate(id)
		if !found {
			return Candidate{}, false
		}
		return normalizeCandidate(candidate), true
	}
	return Candidate{}, false
}

func normalizeCandidates(items []Candidate) []Candidate {
	out := make([]Candidate, 0, len(items))
	for _, item := range items {
		out = append(out, normalizeCandidate(item))
	}
	return out
}

func normalizeCandidate(candidate Candidate) Candidate {
	normalized := candidate
	status := candidate.Status
	if status == "" {
		status = candidate.ReviewStatus
	}
	normalized.Status = normalizeCandidateStatus(status)
	normalized.ReviewStatus = normalized.Status
	return normalized
}

func normalizeCandidateStatus(status string) string {
	switch status {
	case "", legacyCandidateStatusPending:
		return CandidateStatusQueuedForReview
	default:
		return status
	}
}

func isQueuedCandidateStatus(status string) bool {
	switch normalizeCandidateStatus(status) {
	case CandidateStatusQueuedForReview:
		return true
	default:
		return false
	}
}
