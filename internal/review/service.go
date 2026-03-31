package review

import (
	"time"
)

type Candidate struct {
	ID           int64     `json:"id"`
	DatasetID    int64     `json:"dataset_id"`
	SnapshotID   int64     `json:"snapshot_id"`
	ItemID       int64     `json:"item_id"`
	CategoryID   int64     `json:"category_id"`
	ReviewStatus string    `json:"review_status"`
	ReviewerID   string    `json:"reviewer_id,omitempty"`
	ReviewedAt   time.Time `json:"reviewed_at,omitempty"`
}

type Annotation struct {
	CandidateID int64     `json:"candidate_id"`
	ReviewerID  string    `json:"reviewer_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type AuditEvent struct {
	Actor        string    `json:"actor"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	TS           time.Time `json:"ts"`
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
	return items
}

func (s *Service) AcceptCandidate(candidateID int64, reviewer string) error {
	return s.repo.Accept(candidateID, reviewer)
}

func (s *Service) RejectCandidate(candidateID int64, reviewer string) error {
	return s.repo.Reject(candidateID, reviewer)
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
		return repo.GetCandidate(id)
	}
	return Candidate{}, false
}

func (s *Service) PendingCandidateCount(projectID int64) (int, error) {
	if repo, ok := s.repo.(interface {
		PendingCandidateCount(projectID int64) (int, error)
	}); ok {
		return repo.PendingCandidateCount(projectID)
	}
	return len(s.ListCandidates()), nil
}
