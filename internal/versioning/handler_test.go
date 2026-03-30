package versioning

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"yolo-ave-mujica/internal/server"
)

type fakeAnnotationRepository struct {
	bySnapshot map[int64][]Annotation
}

func (r fakeAnnotationRepository) ListEffectiveAnnotations(snapshotID int64) ([]Annotation, error) {
	return append([]Annotation(nil), r.bySnapshot[snapshotID]...), nil
}

func TestDiffReturnsAddRemoveUpdateAndStats(t *testing.T) {
	h := NewHandler(NewService())
	srv := server.NewHTTPServerWithModules(server.Modules{
		Versioning: server.VersioningRoutes{
			DiffSnapshots: h.DiffSnapshots,
		},
	})

	reqBody := DiffRequest{
		Before: []Annotation{
			{ItemID: 1, CategoryID: 1, BBoxX: 0, BBoxY: 0, BBoxW: 10, BBoxH: 10},
			{ItemID: 3, CategoryID: 1, BBoxX: 5, BBoxY: 5, BBoxW: 8, BBoxH: 8},
		},
		After: []Annotation{
			{ItemID: 1, CategoryID: 1, BBoxX: 1, BBoxY: 1, BBoxW: 10, BBoxH: 10}, // update
			{ItemID: 2, CategoryID: 1, BBoxX: 6, BBoxY: 6, BBoxW: 5, BBoxH: 5},   // add
		},
		IOUThreshold: 0.5,
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal req: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/snapshots/diff", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var out DiffResult
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Stats.UpdatedCount == 0 {
		t.Fatalf("expected updates > 0, got %+v", out.Stats)
	}
	if out.Stats.AddedCount == 0 {
		t.Fatalf("expected adds > 0, got %+v", out.Stats)
	}
	if out.Stats.RemovedCount == 0 {
		t.Fatalf("expected removes > 0, got %+v", out.Stats)
	}
}

func TestDiffHandlesMultipleBoxesForSameItemAndCategory(t *testing.T) {
	svc := NewService()

	out := svc.DiffSnapshots(
		[]Annotation{
			{ItemID: 1, CategoryID: 1, BBoxX: 0, BBoxY: 0, BBoxW: 10, BBoxH: 10},
			{ItemID: 1, CategoryID: 1, BBoxX: 30, BBoxY: 30, BBoxW: 8, BBoxH: 8},
		},
		[]Annotation{
			{ItemID: 1, CategoryID: 1, BBoxX: 1, BBoxY: 1, BBoxW: 10, BBoxH: 10},
			{ItemID: 1, CategoryID: 1, BBoxX: 30, BBoxY: 30, BBoxW: 8, BBoxH: 8},
			{ItemID: 1, CategoryID: 1, BBoxX: 50, BBoxY: 50, BBoxW: 6, BBoxH: 6},
		},
		0.5,
	)

	if out.Stats.UpdatedCount != 1 {
		t.Fatalf("expected 1 update, got %+v", out.Stats)
	}
	if out.Stats.AddedCount != 1 {
		t.Fatalf("expected 1 add, got %+v", out.Stats)
	}
	if out.Stats.RemovedCount != 0 {
		t.Fatalf("expected 0 removes, got %+v", out.Stats)
	}
}

func TestDiffReturnsCompatibilityScore(t *testing.T) {
	out := NewService().DiffSnapshots(nil, nil, 0.5)
	if out.CompatibilityScore != 1 {
		t.Fatalf("expected empty snapshots to be fully compatible, got %f", out.CompatibilityScore)
	}
}

func TestDiffBySnapshotIDsUsesRepositoryState(t *testing.T) {
	h := NewHandler(NewServiceWithRepository(fakeAnnotationRepository{
		bySnapshot: map[int64][]Annotation{
			1: {
				{ItemID: 1, CategoryID: 1, BBoxX: 0, BBoxY: 0, BBoxW: 10, BBoxH: 10},
			},
			2: {
				{ItemID: 1, CategoryID: 1, BBoxX: 1, BBoxY: 1, BBoxW: 10, BBoxH: 10},
				{ItemID: 2, CategoryID: 1, BBoxX: 6, BBoxY: 6, BBoxW: 5, BBoxH: 5},
			},
		},
	}))
	srv := server.NewHTTPServerWithModules(server.Modules{
		Versioning: server.VersioningRoutes{
			DiffSnapshots: h.DiffSnapshots,
		},
	})

	b, err := json.Marshal(map[string]any{
		"before_snapshot_id": 1,
		"after_snapshot_id":  2,
		"iou_threshold":      0.5,
	})
	if err != nil {
		t.Fatalf("marshal req: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/snapshots/diff", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var out DiffResult
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Stats.UpdatedCount != 1 || out.Stats.AddedCount != 1 || out.Stats.RemovedCount != 0 {
		t.Fatalf("unexpected diff stats from snapshot ids: %+v", out.Stats)
	}
}
