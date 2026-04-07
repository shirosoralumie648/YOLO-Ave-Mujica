package review

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"yolo-ave-mujica/internal/auth"
	"yolo-ave-mujica/internal/jobs"
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

func (r *fakeRepository) Reject(candidateID int64, _ string, _ string) error {
	r.rejected = append(r.rejected, candidateID)
	return nil
}

func (r *fakeRepository) GetCandidate(id int64) (Candidate, bool) {
	for _, candidate := range r.pending {
		if candidate.ID == id {
			return candidate, true
		}
	}
	return Candidate{}, false
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

func (r *fakeRepository) PersistCandidates(_ int64, items []PersistCandidateInput) ([]Candidate, error) {
	out := make([]Candidate, 0, len(items))
	for idx, item := range items {
		out = append(out, Candidate{
			ID:           int64(idx + 1),
			DatasetID:    item.DatasetID,
			SnapshotID:   item.SnapshotID,
			ItemID:       item.ItemID,
			ObjectKey:    item.ObjectKey,
			CategoryID:   item.CategoryID,
			BBox:         item.BBox,
			Status:       CandidateStatusQueuedForReview,
			ReviewStatus: CandidateStatusQueuedForReview,
		})
	}
	return out, nil
}

type jobsReviewSinkAdapter struct {
	svc *Service
}

func (a jobsReviewSinkAdapter) PersistCandidates(jobID int64, items []jobs.ReviewCandidateInput) ([]jobs.PersistedReviewCandidate, error) {
	inputs := make([]PersistCandidateInput, 0, len(items))
	for _, item := range items {
		inputs = append(inputs, PersistCandidateInput{
			DatasetID:    item.DatasetID,
			SnapshotID:   item.SnapshotID,
			ItemID:       item.ItemID,
			ObjectKey:    item.ObjectKey,
			CategoryID:   item.CategoryID,
			CategoryName: item.CategoryName,
			BBox: CandidateBBox{
				X: item.BBox.X,
				Y: item.BBox.Y,
				W: item.BBox.W,
				H: item.BBox.H,
			},
			Confidence: item.Confidence,
			ModelName:  item.ModelName,
			IsPseudo:   item.IsPseudo,
		})
	}
	persisted, err := a.svc.PersistCandidates(jobID, inputs)
	if err != nil {
		return nil, err
	}
	out := make([]jobs.PersistedReviewCandidate, 0, len(persisted))
	for _, candidate := range persisted {
		out = append(out, jobs.PersistedReviewCandidate{ID: candidate.ID})
	}
	return out, nil
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

func TestListCandidatesIncludesMaterializedJobSourceMetadata(t *testing.T) {
	reviewSvc := NewService()
	jobSvc := jobs.NewServiceWithReviewSink(jobs.NewInMemoryRepository(), nil, jobsReviewSinkAdapter{svc: reviewSvc})

	job, err := jobSvc.CreateJob(jobs.CreateJobInput{
		ProjectID:            1,
		DatasetID:            1,
		SnapshotID:           2,
		JobType:              "zero-shot",
		RequiredResourceType: "gpu",
		IdempotencyKey:       "idem-review-api-results",
		Payload:              map[string]any{"prompt": "person"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	if err := jobSvc.ReportEvent(job.ID, nil, "info", "review_candidates_materialized", "persisted review candidates", map[string]any{
		"result_type":  "annotation_candidates",
		"result_count": 1,
		"candidates": []any{
			map[string]any{
				"dataset_id":    float64(1),
				"snapshot_id":   float64(2),
				"item_id":       float64(9),
				"object_key":    "images/9.jpg",
				"category_id":   float64(3),
				"category_name": "person",
				"confidence":    0.91,
				"model_name":    "grounding-dino-mvp",
				"is_pseudo":     true,
				"bbox": map[string]any{
					"x": 10.0,
					"y": 11.0,
					"w": 12.0,
					"h": 13.0,
				},
			},
		},
	}); err != nil {
		t.Fatalf("report event: %v", err)
	}

	handler := NewHandler(reviewSvc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		Review: server.ReviewRoutes{
			ListCandidates:  handler.ListCandidates,
			AcceptCandidate: handler.AcceptCandidate,
			RejectCandidate: handler.RejectCandidate,
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/review/candidates", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"job_id":1`) {
		t.Fatalf("expected job source metadata, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"model_name":"grounding-dino-mvp"`) {
		t.Fatalf("expected model_name in source metadata, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"confidence":0.91`) {
		t.Fatalf("expected confidence in source metadata, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"is_pseudo":true`) {
		t.Fatalf("expected pseudo source metadata, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"queued_for_review"`) {
		t.Fatalf("expected queued review status, got %s", rec.Body.String())
	}
}

func TestListCandidatesFiltersByCallerProjectScope(t *testing.T) {
	svc := NewService()
	svc.SeedCandidate(Candidate{
		ID:           31,
		ProjectID:    1,
		DatasetID:    1,
		SnapshotID:   1,
		ItemID:       1,
		CategoryID:   1,
		ReviewStatus: CandidateStatusQueuedForReview,
		Status:       CandidateStatusQueuedForReview,
	})
	svc.SeedCandidate(Candidate{
		ID:           32,
		ProjectID:    2,
		DatasetID:    2,
		SnapshotID:   2,
		ItemID:       2,
		CategoryID:   2,
		ReviewStatus: CandidateStatusQueuedForReview,
		Status:       CandidateStatusQueuedForReview,
	})

	handler := NewHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/v1/review/candidates", nil)
	req = req.WithContext(auth.WithIdentity(req.Context(), auth.NewIdentity("reviewer-1", []int64{2})))
	rec := httptest.NewRecorder()

	handler.ListCandidates(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"id":31`) {
		t.Fatalf("expected project 1 candidate to be filtered out, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"id":32`) {
		t.Fatalf("expected project 2 candidate to remain visible, got %s", rec.Body.String())
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

func TestAcceptCandidateRejectsProjectOutsideCallerScope(t *testing.T) {
	svc := NewService()
	svc.SeedCandidate(Candidate{
		ID:           21,
		ProjectID:    1,
		DatasetID:    1,
		SnapshotID:   1,
		ItemID:       1,
		CategoryID:   1,
		ReviewStatus: "pending",
	})

	handler := NewHandler(svc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		MutationMiddleware: auth.IdentityMiddleware([]int64{2}),
		Review: server.ReviewRoutes{
			AcceptCandidate: handler.AcceptCandidate,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/review/candidates/21/accept", strings.NewReader(`{"reviewer_id":"reviewer-1"}`))
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for accept candidate, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "project") {
		t.Fatalf("expected project scope error, got %s", rec.Body.String())
	}
}

func TestRejectCandidatePreservesReviewMetadata(t *testing.T) {
	svc := NewService()
	svc.SeedCandidate(Candidate{ID: 11, DatasetID: 1, SnapshotID: 1, ItemID: 1, CategoryID: 1, ReviewStatus: "pending"})

	if err := svc.RejectCandidate(11, "reviewer-1", "low_confidence"); err != nil {
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

func TestRejectCandidateRejectsProjectOutsideCallerScope(t *testing.T) {
	svc := NewService()
	svc.SeedCandidate(Candidate{
		ID:           22,
		ProjectID:    1,
		DatasetID:    1,
		SnapshotID:   1,
		ItemID:       1,
		CategoryID:   1,
		ReviewStatus: "pending",
	})

	handler := NewHandler(svc)
	srv := server.NewHTTPServerWithModules(server.Modules{
		MutationMiddleware: auth.IdentityMiddleware([]int64{2}),
		Review: server.ReviewRoutes{
			RejectCandidate: handler.RejectCandidate,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/review/candidates/22/reject", strings.NewReader(`{"reviewer_id":"reviewer-1","reason_code":"out-of-scope"}`))
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for reject candidate, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRejectCandidateRequiresReasonCode(t *testing.T) {
	svc := NewService()
	svc.SeedCandidate(Candidate{ID: 13, DatasetID: 1, SnapshotID: 1, ItemID: 1, CategoryID: 1, ReviewStatus: "pending"})
	handler := NewHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/v1/review/candidates/13/reject", strings.NewReader(`{"reviewer_id":"u1"}`))
	rec := httptest.NewRecorder()

	srv := server.NewHTTPServerWithModules(server.Modules{
		Review: server.ReviewRoutes{
			ListCandidates:  handler.ListCandidates,
			AcceptCandidate: handler.AcceptCandidate,
			RejectCandidate: handler.RejectCandidate,
		},
	})
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "reason_code is required") {
		t.Fatalf("expected reason_code validation error, got %s", rec.Body.String())
	}
}

func TestRejectCandidateRecordsReasonCodeInAudit(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewServiceWithRepository(repo)
	svc.SeedCandidate(Candidate{ID: 14, DatasetID: 1, SnapshotID: 1, ItemID: 1, CategoryID: 1, ReviewStatus: "pending"})

	if err := svc.RejectCandidate(14, "reviewer-1", "box_out_of_scope"); err != nil {
		t.Fatalf("reject candidate: %v", err)
	}
	if len(repo.audits) != 1 {
		t.Fatalf("expected one audit row, got %d", len(repo.audits))
	}
	if repo.audits[0].Detail["reason_code"] != "box_out_of_scope" {
		t.Fatalf("expected reason_code in audit detail, got %+v", repo.audits[0].Detail)
	}
}
