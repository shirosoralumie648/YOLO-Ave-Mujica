# AI Candidate Lifecycle And Review Metadata Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Align annotation review candidates with the product spec AI-candidate lifecycle and expose source metadata needed by review consumers without breaking existing publish and task flows.

**Architecture:** Keep the existing `internal/review` surface area and route shape, but normalize legacy `pending` rows into the spec-aligned `queued_for_review` state at the service boundary. Extend candidate payloads with explicit source metadata from `annotation_candidates`, keep accept/reject persistence compatible with old rows, and preserve `accepted`-based downstream publish behavior.

**Tech Stack:** Go 1.24, Chi, pgx/v5, PostgreSQL, OpenAPI, Go test.

---

## Scope Check

This plan covers a single Phase 3B backend slice:

1. normalize review candidate queue state from legacy `pending` to spec-facing `queued_for_review`
2. allow accept/reject transitions from both legacy and new queued states
3. expose candidate source metadata and provenance fields in review payloads
4. document the updated contract in `api/openapi/mvp.yaml`

This slice does **not** cover:

1. review UI changes
2. reject `reason_code` persistence
3. feedback bus ingestion
4. video timeline/runtime work

## File Structure

**Create**

- `docs/superpowers/plans/2026-04-03-ai-candidate-lifecycle-review-metadata-plan.md`

**Modify**

- `internal/review/service.go`
- `internal/review/repository.go`
- `internal/review/postgres_repository.go`
- `internal/review/handler_test.go`
- `internal/review/postgres_repository_test.go`
- `cmd/api-server/main_test.go`
- `api/openapi/mvp.yaml`

## Task 1: Lock the contract with failing tests

**Files:**
- Modify: `internal/review/handler_test.go`
- Modify: `cmd/api-server/main_test.go`
- Modify: `internal/review/postgres_repository_test.go`

- [ ] **Step 1: Write failing handler/service tests for normalized queue state and source metadata**

```go
func TestServiceListCandidatesNormalizesQueuedStateAndSourceMetadata(t *testing.T) {
	repo := &fakeRepository{
		pending: []Candidate{{
			ID:           12,
			DatasetID:    1,
			SnapshotID:   1,
			ItemID:       1,
			CategoryID:   1,
			ReviewStatus: "pending",
			JobID:        ptrInt64(91),
			Confidence:   ptrFloat64(0.82),
			ModelName:    "detector-a",
			IsPseudo:     true,
		}},
	}

	svc := NewServiceWithRepository(repo)
	items := svc.ListCandidates()
	if items[0].Status != "queued_for_review" {
		t.Fatalf("expected queued_for_review, got %+v", items[0])
	}
	if items[0].Source.ModelName != "detector-a" {
		t.Fatalf("expected source metadata, got %+v", items[0].Source)
	}
}
```

- [ ] **Step 2: Run the focused tests to verify they fail**

Run: `env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/review ./cmd/api-server -run 'TestServiceListCandidatesNormalizesQueuedStateAndSourceMetadata|TestBuildModulesWithHandlersUsesInjectedReviewPublishAndArtifacts' -count=1`

Expected: FAIL because `Candidate` does not yet expose normalized `status` or `source` metadata.

- [ ] **Step 3: Write failing postgres repository coverage for legacy/new queued states**

```go
func TestPostgresRepositoryListPendingNormalizesLegacyAndQueuedRows(t *testing.T) {
	// seed one `pending` row and one `queued_for_review` row
	// expect both to be returned to the review queue
}
```

- [ ] **Step 4: Run the postgres test to verify it fails**

Run: `env INTEGRATION_DATABASE_URL=postgres://... GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/review -run TestPostgresRepositoryListPendingNormalizesLegacyAndQueuedRows -count=1`

Expected: FAIL because the SQL still filters only `pending` and omits metadata fields.

## Task 2: Implement normalized lifecycle and source metadata

**Files:**
- Modify: `internal/review/service.go`
- Modify: `internal/review/repository.go`
- Modify: `internal/review/postgres_repository.go`

- [ ] **Step 1: Add spec-facing state and source metadata to `Candidate`**

```go
type CandidateSource struct {
	JobID      *int64   `json:"job_id,omitempty"`
	Confidence *float64 `json:"confidence,omitempty"`
	ModelName  string   `json:"model_name,omitempty"`
	IsPseudo   bool     `json:"is_pseudo"`
}

type Candidate struct {
	// existing identifiers...
	Status       string          `json:"status"`
	ReviewStatus string          `json:"review_status"`
	Source       CandidateSource `json:"source"`
}
```

- [ ] **Step 2: Normalize legacy queue rows at the service/repository boundary**

```go
func normalizeCandidateStatus(raw string) string {
	switch raw {
	case "", "pending", "generated":
		return "queued_for_review"
	default:
		return raw
	}
}
```

- [ ] **Step 3: Accept/reject from both legacy and new queued states while writing spec-facing values going forward**

```go
func isQueuedCandidateStatus(status string) bool {
	switch status {
	case "pending", "queued_for_review":
		return true
	default:
		return false
	}
}
```

- [ ] **Step 4: Extend postgres queries to select provenance columns and include both queue states**

```sql
select id, job_id, dataset_id, snapshot_id, item_id, category_id,
       confidence, model_name, is_pseudo, review_status, reviewer_id, reviewed_at, created_at
from annotation_candidates
where review_status in ('pending', 'queued_for_review')
order by id asc
```

- [ ] **Step 5: Keep publish queries filtering `accepted` rows**

```go
where d.project_id = $1 and c.review_status = 'accepted'
```

## Task 3: Update contract docs and verify

**Files:**
- Modify: `api/openapi/mvp.yaml`

- [ ] **Step 1: Expand the review candidate route documentation**

```yaml
/v1/review/candidates:
  get:
    summary: List queued review candidates with source metadata
```

- [ ] **Step 2: Run focused verification**

Run: `env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/review ./cmd/api-server -count=1`

Expected: PASS

- [ ] **Step 3: Run broader affected-package verification**

Run: `env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/review ./internal/publish ./internal/server ./cmd/api-server -count=1`

Expected: PASS
