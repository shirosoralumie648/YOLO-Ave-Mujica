package review

import (
	"fmt"
	"sync"
	"time"
)

type Repository interface {
	ListPending() ([]Candidate, error)
	Accept(candidateID int64, reviewer string) error
	Reject(candidateID int64, reviewer string) error
}

type InMemoryRepository struct {
	mu          sync.Mutex
	candidates  map[int64]Candidate
	annotations []Annotation
	audits      []AuditEvent
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		candidates:  make(map[int64]Candidate),
		annotations: []Annotation{},
		audits:      []AuditEvent{},
	}
}

func (r *InMemoryRepository) SeedCandidate(c Candidate) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.ReviewStatus == "" {
		c.ReviewStatus = "pending"
	}
	r.candidates[c.ID] = c
}

func (r *InMemoryRepository) ListPending() ([]Candidate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]Candidate, 0, len(r.candidates))
	for _, c := range r.candidates {
		if c.ReviewStatus == "pending" {
			out = append(out, c)
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
	if c.ReviewStatus != "pending" {
		return fmt.Errorf("candidate is %s", c.ReviewStatus)
	}

	now := time.Now().UTC()
	c.ReviewStatus = "accepted"
	c.ReviewerID = reviewer
	c.ReviewedAt = now
	r.candidates[candidateID] = c
	r.annotations = append(r.annotations, Annotation{CandidateID: candidateID, ReviewerID: reviewer, CreatedAt: now})
	r.audits = append(r.audits, AuditEvent{Actor: reviewer, Action: "review.accept", ResourceType: "annotation_candidate", ResourceID: fmt.Sprintf("%d", candidateID), TS: now})
	return nil
}

func (r *InMemoryRepository) Reject(candidateID int64, reviewer string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.candidates[candidateID]
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
	r.candidates[candidateID] = c
	r.audits = append(r.audits, AuditEvent{Actor: reviewer, Action: "review.reject", ResourceType: "annotation_candidate", ResourceID: fmt.Sprintf("%d", candidateID), TS: now})
	return nil
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
	return c, ok
}

var _ Repository = (*InMemoryRepository)(nil)
