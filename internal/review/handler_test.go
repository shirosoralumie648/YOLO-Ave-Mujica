package review

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"yolo-ave-mujica/internal/server"
)

type fakeRepository struct {
	pending  []Candidate
	accepted []int64
	rejected []int64
}

func ptrInt64(v int64) *int64 {
	return &v
}

func ptrFloat64(v float64) *float64 {
	return &v
}

func (r *fakeRepository) ListPending() ([]Candidate, error) {
	out := make([]Candidate, len(r.pending))
	copy(out, r.pending)
	return out, nil
}

func (r *fakeRepository) Accept(candidateID int64, _ string) error {
	r.accepted = append(r.accepted, candidateID)
	return nil
}

func (r *fakeRepository) Reject(candidateID int64, _ string) error {
	r.rejected = append(r.rejected, candidateID)
	return nil
}

func (r *fakeRepository) ListPublishableCandidates(projectID int64) ([]PublishableCandidate, error) {
	return []PublishableCandidate{
		{
			ID:           1,
			ProjectID:    projectID,
			DatasetID:    1,
			SnapshotID:   1,
			ItemID:       1,
			TaskID:       1,
			ReviewStatus: "accepted",
			RiskLevel:    "normal",
		},
	}, nil
}

func TestServiceUsesRepositoryPendingCandidates(t *testing.T) {
	repo := &fakeRepository{
		pending: []Candidate{{
			ID:           12,
			DatasetID:    1,
			SnapshotID:   1,
			ItemID:       1,
			CategoryID:   1,
			ReviewStatus: "pending",
			Status:       "queued_for_review",
			Source: CandidateSource{
				JobID:      ptrInt64(91),
				Confidence: ptrFloat64(0.82),
				ModelName:  "detector-a",
				IsPseudo:   true,
			},
		}},
	}

	svc := NewServiceWithRepository(repo)
	items := svc.ListCandidates()
	if len(items) != 1 || items[0].ID != 12 {
		t.Fatalf("expected repository-backed pending candidates, got %+v", items)
	}
	if items[0].Status != "queued_for_review" || items[0].ReviewStatus != "queued_for_review" {
		t.Fatalf("expected normalized queued status, got %+v", items[0])
	}
	if items[0].Source.JobID == nil || *items[0].Source.JobID != 91 {
		t.Fatalf("expected source job metadata, got %+v", items[0].Source)
	}
	if items[0].Source.Confidence == nil || *items[0].Source.Confidence != 0.82 {
		t.Fatalf("expected source confidence metadata, got %+v", items[0].Source)
	}
	if items[0].Source.ModelName != "detector-a" || !items[0].Source.IsPseudo {
		t.Fatalf("expected source model metadata, got %+v", items[0].Source)
	}
}

func TestAcceptPromotesCandidateToAnnotation(t *testing.T) {
	svc := NewService()
	svc.SeedCandidate(Candidate{
		ID:           10,
		DatasetID:    1,
		SnapshotID:   1,
		ItemID:       1,
		CategoryID:   1,
		ReviewStatus: "pending",
	})
	h := NewHandler(svc)

	srv := server.NewHTTPServerWithModules(server.Modules{
		Review: server.ReviewRoutes{
			ListCandidates:  h.ListCandidates,
			AcceptCandidate: h.AcceptCandidate,
			RejectCandidate: h.RejectCandidate,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/review/candidates/10/accept", strings.NewReader(`{"reviewer_id":"u1"}`))
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if svc.AnnotationCount() != 1 {
		t.Fatalf("expected one promoted annotation, got %d", svc.AnnotationCount())
	}
	c, ok := svc.GetCandidate(10)
	if !ok {
		t.Fatal("candidate 10 missing")
	}
	if c.Status != "accepted" || c.ReviewStatus != "accepted" {
		t.Fatalf("expected accepted, got %+v", c)
	}
}

func TestRejectCandidatePreservesReviewMetadata(t *testing.T) {
	svc := NewService()
	svc.SeedCandidate(Candidate{ID: 11, DatasetID: 1, SnapshotID: 1, ItemID: 1, CategoryID: 1, ReviewStatus: "pending"})

	if err := svc.RejectCandidate(11, "reviewer-1"); err != nil {
		t.Fatalf("reject candidate: %v", err)
	}
	c, ok := svc.GetCandidate(11)
	if !ok {
		t.Fatal("candidate 11 missing")
	}
	if c.ReviewerID != "reviewer-1" || c.Status != "rejected" || c.ReviewStatus != "rejected" {
		t.Fatalf("unexpected candidate state: %+v", c)
	}
	if c.ReviewedAt.IsZero() {
		t.Fatalf("expected reviewed timestamp, got zero")
	}
}
