package review

import (
	"fmt"
	"sync"
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
	mu          sync.Mutex
	candidates  map[int64]Candidate
	annotations []Annotation
	audits      []AuditEvent
}

func NewService() *Service {
	return &Service{
		candidates:  make(map[int64]Candidate),
		annotations: []Annotation{},
		audits:      []AuditEvent{},
	}
}

func (s *Service) SeedCandidate(c Candidate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.ReviewStatus == "" {
		c.ReviewStatus = "pending"
	}
	s.candidates[c.ID] = c
}

func (s *Service) ListCandidates() []Candidate {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Candidate, 0, len(s.candidates))
	for _, c := range s.candidates {
		if c.ReviewStatus == "pending" {
			out = append(out, c)
		}
	}
	return out
}

func (s *Service) AcceptCandidate(candidateID int64, reviewer string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.candidates[candidateID]
	if !ok {
		return fmt.Errorf("candidate %d not found", candidateID)
	}
	if c.ReviewStatus != "pending" {
		return fmt.Errorf("candidate is %s", c.ReviewStatus)
	}

	now := time.Now().UTC()
	c.ReviewStatus = "accepted"
	c.ReviewerID = reviewer
	c.ReviewedAt = now
	s.candidates[candidateID] = c
	s.annotations = append(s.annotations, Annotation{CandidateID: candidateID, ReviewerID: reviewer, CreatedAt: now})
	s.audits = append(s.audits, AuditEvent{Actor: reviewer, Action: "review.accept", ResourceType: "annotation_candidate", ResourceID: fmt.Sprintf("%d", candidateID), TS: now})
	return nil
}

func (s *Service) RejectCandidate(candidateID int64, reviewer string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.candidates[candidateID]
	if !ok {
		return fmt.Errorf("candidate %d not found", candidateID)
	}
	if c.ReviewStatus != "pending" {
		return fmt.Errorf("candidate is %s", c.ReviewStatus)
	}

	now := time.Now().UTC()
	c.ReviewStatus = "rejected"
	c.ReviewerID = reviewer
	c.ReviewedAt = now
	s.candidates[candidateID] = c
	s.audits = append(s.audits, AuditEvent{Actor: reviewer, Action: "review.reject", ResourceType: "annotation_candidate", ResourceID: fmt.Sprintf("%d", candidateID), TS: now})
	return nil
}

func (s *Service) AnnotationCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.annotations)
}

func (s *Service) GetCandidate(id int64) (Candidate, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.candidates[id]
	return c, ok
}
