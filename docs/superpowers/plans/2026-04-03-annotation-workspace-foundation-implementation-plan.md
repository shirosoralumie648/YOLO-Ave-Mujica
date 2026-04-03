# Annotation Workspace Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the first executable Phase 3 slice by adding annotation draft persistence, task-bound asset context, an annotator workspace page, and the autosave/submit API loop that moves work from `in_progress` into `submitted`.

**Architecture:** Extend the existing `internal/tasks` kernel so annotation tasks carry explicit asset context and the richer task states required by the approved product spec. Add a focused `internal/annotations` domain that owns draft documents and workspace aggregation, then build an image-first web workspace that reads a single aggregated payload, saves drafts through one endpoint, and submits through one endpoint. Defer AI candidate lifecycle, review-source metadata, and video timeline/runtime work to follow-up plans so this slice stays small enough to execute safely.

**Tech Stack:** Go 1.24, Chi, pgx/v5, PostgreSQL, React 19, TypeScript, React Router, TanStack Query, Vitest, Testing Library, bash smoke checks.

---

## Scope Check

This plan intentionally covers **Phase 3A only**:

1. annotation task state alignment
2. draft annotation persistence
3. workspace aggregate API
4. annotator-facing workspace shell
5. autosave and submit flow

This plan does **not** include:

1. AI candidate accept/edit/ignore actions
2. source model metadata in review mode
3. video timeline, interpolation, or persistence checker
4. publish-gate changes
5. training/evaluation flows

Follow-up decomposition after this slice:

1. Phase 3B: AI candidate lifecycle and review-source metadata
2. Phase 3C: video/frame durability, timeline strip, persistence checker

## Migration Numbering Note

The repository currently ends at `000004_publish_gate_review_workspace`. This slice should consume `000005_*` for annotation workspace foundation. When the training/evaluation phase is planned later, its migration should move from the historical `000005_*` placeholder to `000006_*`.

## File Structure

### Backend

**Create**

- `migrations/000005_annotation_workspace_foundation.up.sql`
- `migrations/000005_annotation_workspace_foundation.down.sql`
- `internal/annotations/model.go`
- `internal/annotations/repository.go`
- `internal/annotations/in_memory_repository.go`
- `internal/annotations/postgres_repository.go`
- `internal/annotations/service.go`
- `internal/annotations/handler.go`
- `internal/annotations/service_test.go`
- `internal/annotations/handler_test.go`
- `internal/annotations/postgres_repository_test.go`

**Modify**

- `internal/tasks/model.go`
- `internal/tasks/repository.go`
- `internal/tasks/postgres_repository.go`
- `internal/tasks/service.go`
- `internal/tasks/service_test.go`
- `internal/tasks/handler.go`
- `internal/tasks/handler_test.go`
- `internal/server/http_server.go`
- `internal/server/http_server_routes_test.go`
- `cmd/api-server/main.go`
- `cmd/api-server/main_test.go`
- `api/openapi/mvp.yaml`
- `scripts/dev/smoke.sh`
- `scripts/dev/smoke_test.go`

### Frontend

**Create**

- `apps/web/src/features/workspace/api.ts`
- `apps/web/src/features/workspace/annotation-workspace-page.tsx`
- `apps/web/src/features/workspace/annotation-workspace.test.tsx`

**Modify**

- `apps/web/src/app/router.tsx`
- `apps/web/src/app/layout/app-shell.tsx`
- `apps/web/src/app/styles.css`
- `apps/web/src/features/tasks/task-detail-page.tsx`
- `apps/web/src/features/tasks/task-detail-page.test.tsx`
- `apps/web/src/features/data/api.ts`

## Contract Shape

### Task Context Additions

Annotation tasks need enough context to open a workspace without guessing:

```go
type Task struct {
	ID              int64      `json:"id"`
	ProjectID       int64      `json:"project_id"`
	SnapshotID      *int64     `json:"snapshot_id,omitempty"`
	Title           string     `json:"title"`
	Kind            string     `json:"kind"`
	Status          string     `json:"status"`
	Priority        string     `json:"priority"`
	Assignee        string     `json:"assignee"`
	AssetObjectKey  string     `json:"asset_object_key"`
	MediaKind       string     `json:"media_kind"`
	FrameIndex      *int       `json:"frame_index,omitempty"`
	OntologyVersion string     `json:"ontology_version"`
	DueAt           *time.Time `json:"due_at,omitempty"`
	BlockerReason   string     `json:"blocker_reason"`
	LastActivityAt  time.Time  `json:"last_activity_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
```

### Annotation Workspace API

This slice uses three endpoints:

1. `GET /v1/tasks/{id}/workspace`
2. `PUT /v1/tasks/{id}/workspace/draft`
3. `POST /v1/tasks/{id}/workspace/submit`

Target response shape:

```json
{
  "task": {
    "id": 18,
    "status": "in_progress",
    "asset_object_key": "train/images/a.jpg",
    "media_kind": "image",
    "ontology_version": "v1"
  },
  "asset": {
    "dataset_id": 1,
    "dataset_name": "yard-ops",
    "snapshot_id": 7,
    "snapshot_version": "v7",
    "object_key": "train/images/a.jpg",
    "frame_index": null
  },
  "draft": {
    "id": 31,
    "task_id": 18,
    "state": "draft",
    "revision": 4,
    "body": {
      "objects": [
        { "id": "box-1", "label": "person", "x": 0.42, "y": 0.38, "w": 0.22, "h": 0.44 }
      ]
    },
    "updated_at": "2026-04-03T08:00:00Z"
  }
}
```

## Task 1: Align Task Lifecycle And Asset Context

**Files:**

- Create: `migrations/000005_annotation_workspace_foundation.up.sql`
- Create: `migrations/000005_annotation_workspace_foundation.down.sql`
- Modify: `internal/tasks/model.go`
- Modify: `internal/tasks/repository.go`
- Modify: `internal/tasks/postgres_repository.go`
- Modify: `internal/tasks/service.go`
- Modify: `internal/tasks/service_test.go`
- Modify: `internal/tasks/handler.go`
- Modify: `internal/tasks/handler_test.go`

- [ ] **Step 1: Write the failing task tests for richer status transitions and asset context**

Append to `internal/tasks/service_test.go`:

```go
func TestServiceTransitionTaskSupportsSubmittedLifecycle(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewInMemoryRepository())

	task, err := svc.CreateTask(ctx, CreateTaskInput{
		ProjectID:      1,
		Title:          "Annotate yard frame A",
		Assignee:       "annotator-1",
		AssetObjectKey: "train/images/a.jpg",
		MediaKind:      MediaKindImage,
		Status:         StatusReady,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	task, err = svc.TransitionTask(ctx, task.ID, TransitionTaskInput{Status: StatusInProgress})
	if err != nil {
		t.Fatalf("start task: %v", err)
	}
	task, err = svc.TransitionTask(ctx, task.ID, TransitionTaskInput{Status: StatusSubmitted})
	if err != nil {
		t.Fatalf("submit task: %v", err)
	}
	if task.Status != StatusSubmitted {
		t.Fatalf("expected submitted, got %s", task.Status)
	}
}

func TestServiceCreateTaskRequiresAssetContextForAnnotationTasks(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewInMemoryRepository())

	_, err := svc.CreateTask(ctx, CreateTaskInput{
		ProjectID: 1,
		Title:     "Annotate yard frame B",
		Kind:      KindAnnotation,
	})
	if err == nil {
		t.Fatal("expected annotation task without asset context to fail")
	}
}
```

- [ ] **Step 2: Run the targeted task tests and confirm RED**

Run:

```bash
env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/tasks -run 'TestServiceTransitionTaskSupportsSubmittedLifecycle|TestServiceCreateTaskRequiresAssetContextForAnnotationTasks' -count=1 -v
```

Expected:

```text
FAIL ... invalid status "submitted"
FAIL ... expected annotation task without asset context to fail
```

- [ ] **Step 3: Implement the minimal task schema and lifecycle changes**

Update `migrations/000005_annotation_workspace_foundation.up.sql` and the task model/service so annotation tasks can carry asset context and move into `submitted`:

```sql
alter table tasks
  add column asset_object_key text not null default '',
  add column media_kind text not null default 'image',
  add column frame_index integer,
  add column ontology_version text not null default 'v1';

alter table tasks
  drop constraint tasks_status_check,
  add constraint tasks_status_check check (
    status in (
      'queued', 'ready', 'in_progress', 'blocked',
      'submitted', 'reviewing', 'rework_required',
      'accepted', 'published', 'closed'
    )
  );
```

```go
const (
	StatusSubmitted      = "submitted"
	StatusReviewing      = "reviewing"
	StatusReworkRequired = "rework_required"
	StatusAccepted       = "accepted"
	StatusPublished      = "published"
	StatusClosed         = "closed"
)

const (
	MediaKindImage = "image"
	MediaKindVideo = "video"
)
```

```go
if in.Kind == KindAnnotation && strings.TrimSpace(in.AssetObjectKey) == "" {
	return Task{}, fmt.Errorf("asset_object_key is required for annotation tasks")
}
if in.Kind == KindAnnotation && !isValidMediaKind(in.MediaKind) {
	return Task{}, fmt.Errorf("invalid media_kind %q", in.MediaKind)
}
```

- [ ] **Step 4: Re-run the full task package and make it green**

Run:

```bash
env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/tasks -count=1 -v
```

Expected:

```text
PASS
ok  	yolo-ave-mujica/internal/tasks	...
```

- [ ] **Step 5: Commit the task-kernel upgrade**

```bash
git add migrations/000005_annotation_workspace_foundation.up.sql migrations/000005_annotation_workspace_foundation.down.sql internal/tasks/model.go internal/tasks/repository.go internal/tasks/postgres_repository.go internal/tasks/service.go internal/tasks/service_test.go internal/tasks/handler.go internal/tasks/handler_test.go
git commit -m "feat: align task lifecycle for annotation workspace"
```

## Task 2: Add Annotation Draft Persistence And Service Rules

**Files:**

- Create: `internal/annotations/model.go`
- Create: `internal/annotations/repository.go`
- Create: `internal/annotations/in_memory_repository.go`
- Create: `internal/annotations/postgres_repository.go`
- Create: `internal/annotations/service.go`
- Create: `internal/annotations/service_test.go`
- Create: `internal/annotations/postgres_repository_test.go`

- [ ] **Step 1: Write the failing annotation service and repository tests**

Create `internal/annotations/service_test.go`:

```go
func TestServiceSaveDraftIncrementsRevision(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewInMemoryRepository(), nil)

	first, err := svc.SaveDraft(ctx, SaveDraftInput{
		TaskID:          18,
		Actor:           "annotator-1",
		SnapshotID:      7,
		AssetObjectKey:  "train/images/a.jpg",
		OntologyVersion: "v1",
		Body: map[string]any{
			"objects": []map[string]any{{"id": "box-1", "label": "person"}},
		},
	})
	if err != nil {
		t.Fatalf("save draft #1: %v", err)
	}
	second, err := svc.SaveDraft(ctx, SaveDraftInput{
		TaskID:          18,
		Actor:           "annotator-1",
		SnapshotID:      7,
		AssetObjectKey:  "train/images/a.jpg",
		OntologyVersion: "v1",
		BaseRevision:    first.Revision,
		Body: map[string]any{
			"objects": []map[string]any{{"id": "box-1", "label": "person"}, {"id": "box-2", "label": "car"}},
		},
	})
	if err != nil {
		t.Fatalf("save draft #2: %v", err)
	}
	if second.Revision != first.Revision+1 {
		t.Fatalf("expected revision increment, got %d -> %d", first.Revision, second.Revision)
	}
}

func TestServiceSubmitMarksDraftSubmitted(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewInMemoryRepository(), nil)

	draft, err := svc.SaveDraft(ctx, SaveDraftInput{
		TaskID:          18,
		Actor:           "annotator-1",
		SnapshotID:      7,
		AssetObjectKey:  "train/images/a.jpg",
		OntologyVersion: "v1",
		Body:            map[string]any{"objects": []any{}},
	})
	if err != nil {
		t.Fatalf("save draft: %v", err)
	}
	submitted, err := svc.Submit(ctx, SubmitInput{TaskID: draft.TaskID, Actor: "annotator-1"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if submitted.State != StateSubmitted {
		t.Fatalf("expected submitted state, got %s", submitted.State)
	}
}
```

- [ ] **Step 2: Run the annotation package tests and confirm RED**

Run:

```bash
env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/annotations -count=1 -v
```

Expected:

```text
FAIL ... undefined: NewService
FAIL ... undefined: SaveDraftInput
```

- [ ] **Step 3: Implement the draft model, repository, and service**

Use this minimal model in `internal/annotations/model.go`:

```go
const (
	StateDraft     = "draft"
	StateSubmitted = "submitted"
)

type Annotation struct {
	ID              int64          `json:"id"`
	TaskID          int64          `json:"task_id"`
	SnapshotID      int64          `json:"snapshot_id"`
	AssetObjectKey  string         `json:"asset_object_key"`
	FrameIndex      *int           `json:"frame_index,omitempty"`
	OntologyVersion string         `json:"ontology_version"`
	State           string         `json:"state"`
	Revision        int64          `json:"revision"`
	Body            map[string]any `json:"body"`
	SubmittedBy     string         `json:"submitted_by"`
	SubmittedAt     *time.Time     `json:"submitted_at,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}
```

Persist one mutable draft row per task in `internal/annotations/postgres_repository.go`:

```sql
create table task_annotations (
  id bigserial primary key,
  task_id bigint not null references tasks(id) on delete cascade,
  snapshot_id bigint not null references dataset_snapshots(id) on delete cascade,
  asset_object_key text not null,
  frame_index integer,
  ontology_version text not null default 'v1',
  state text not null default 'draft',
  revision bigint not null default 1,
  body_json jsonb not null default '{}'::jsonb,
  submitted_by text not null default '',
  submitted_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (task_id)
);
```

- [ ] **Step 4: Re-run the annotation package and keep it green**

Run:

```bash
env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/annotations -count=1 -v
```

Expected:

```text
PASS
ok  	yolo-ave-mujica/internal/annotations	...
```

- [ ] **Step 5: Commit the annotation domain**

```bash
git add internal/annotations/model.go internal/annotations/repository.go internal/annotations/in_memory_repository.go internal/annotations/postgres_repository.go internal/annotations/service.go internal/annotations/service_test.go internal/annotations/postgres_repository_test.go
git commit -m "feat: add annotation draft persistence"
```

## Task 3: Expose Workspace Aggregate, Autosave, And Submit APIs

**Files:**

- Create: `internal/annotations/handler.go`
- Create: `internal/annotations/handler_test.go`
- Modify: `internal/server/http_server.go`
- Modify: `internal/server/http_server_routes_test.go`
- Modify: `cmd/api-server/main.go`
- Modify: `cmd/api-server/main_test.go`
- Modify: `api/openapi/mvp.yaml`

- [ ] **Step 1: Write the failing handler tests for workspace load, draft save, and submit**

Create `internal/annotations/handler_test.go`:

```go
func TestHandlerGetWorkspaceReturnsTaskAndDraft(t *testing.T) {
	svc := newStubService()
	handler := NewHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/v1/tasks/18/workspace", nil)
	rec := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Get("/v1/tasks/{id}/workspace", handler.GetWorkspace)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "\"asset_object_key\":\"train/images/a.jpg\"") {
		t.Fatalf("expected workspace asset context, got %s", rec.Body.String())
	}
}

func TestHandlerSubmitTransitionsTaskToSubmitted(t *testing.T) {
	svc := newStubService()
	handler := NewHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/18/workspace/submit", strings.NewReader(`{"actor":"annotator-1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Post("/v1/tasks/{id}/workspace/submit", handler.SubmitWorkspace)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "\"status\":\"submitted\"") {
		t.Fatalf("expected submitted response, got %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run the annotation and server route tests and confirm RED**

Run:

```bash
env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/annotations ./internal/server ./cmd/api-server -run 'TestHandlerGetWorkspaceReturnsTaskAndDraft|TestHandlerSubmitTransitionsTaskToSubmitted|TestMVPRoutesAreRegistered|TestBuildModulesWithHandlersUsesInjectedReviewPublishAndArtifacts' -count=1 -v
```

Expected:

```text
FAIL ... undefined: NewHandler
FAIL ... route not registered
```

- [ ] **Step 3: Implement the aggregate endpoints and wire them into the server**

Use a page-oriented workspace payload:

```go
type Workspace struct {
	Task  tasks.Task  `json:"task"`
	Asset Asset       `json:"asset"`
	Draft Annotation  `json:"draft"`
}

func (h *Handler) Register(r chi.Router) {
	r.Get("/v1/tasks/{id}/workspace", h.GetWorkspace)
	r.Put("/v1/tasks/{id}/workspace/draft", h.SaveDraft)
	r.Post("/v1/tasks/{id}/workspace/submit", h.SubmitWorkspace)
}
```

Submit must coordinate annotation and task state:

```go
annotation, err := s.repo.Submit(ctx, in.TaskID, in.Actor)
if err != nil {
	return Workspace{}, err
}
task, err := s.taskService.TransitionTask(ctx, in.TaskID, tasks.TransitionTaskInput{Status: tasks.StatusSubmitted})
if err != nil {
	return Workspace{}, err
}
return s.buildWorkspace(ctx, task, annotation)
```

- [ ] **Step 4: Re-run backend tests for annotations, tasks, server, and api-server**

Run:

```bash
env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/annotations ./internal/tasks ./internal/server ./cmd/api-server -count=1 -v
```

Expected:

```text
PASS
ok  	yolo-ave-mujica/internal/annotations	...
ok  	yolo-ave-mujica/internal/tasks	...
ok  	yolo-ave-mujica/internal/server	...
ok  	yolo-ave-mujica/cmd/api-server	...
```

- [ ] **Step 5: Commit the workspace API**

```bash
git add internal/annotations/handler.go internal/annotations/handler_test.go internal/server/http_server.go internal/server/http_server_routes_test.go cmd/api-server/main.go cmd/api-server/main_test.go api/openapi/mvp.yaml
git commit -m "feat: add annotation workspace APIs"
```

## Task 4: Build The Annotator Workspace Page

**Files:**

- Create: `apps/web/src/features/workspace/api.ts`
- Create: `apps/web/src/features/workspace/annotation-workspace-page.tsx`
- Create: `apps/web/src/features/workspace/annotation-workspace.test.tsx`
- Modify: `apps/web/src/app/router.tsx`
- Modify: `apps/web/src/app/layout/app-shell.tsx`
- Modify: `apps/web/src/app/styles.css`
- Modify: `apps/web/src/features/tasks/task-detail-page.tsx`
- Modify: `apps/web/src/features/tasks/task-detail-page.test.tsx`
- Modify: `apps/web/src/features/data/api.ts`

- [ ] **Step 1: Write the failing workspace page tests**

Create `apps/web/src/features/workspace/annotation-workspace.test.tsx`:

```tsx
it("loads workspace data, saves draft, and submits the task", async () => {
  server.use(
    http.get("http://localhost:8080/v1/tasks/18/workspace", () =>
      HttpResponse.json({
        task: { id: 18, status: "in_progress", asset_object_key: "train/images/a.jpg", media_kind: "image" },
        asset: { dataset_id: 1, object_key: "train/images/a.jpg", snapshot_version: "v7" },
        draft: { id: 31, revision: 2, state: "draft", body: { objects: [{ id: "box-1", label: "person" }] } }
      })
    ),
    http.put("http://localhost:8080/v1/tasks/18/workspace/draft", () => HttpResponse.json({ revision: 3 })),
    http.post("http://localhost:8080/v1/tasks/18/workspace/submit", () =>
      HttpResponse.json({ task: { id: 18, status: "submitted" } })
    )
  );

  renderWithRouter(<AnnotationWorkspacePage />, { route: "/tasks/18/workspace" });

  expect(await screen.findByText("Annotation Workspace")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Save Draft" }));
  await userEvent.click(screen.getByRole("button", { name: "Submit Task" }));
  expect(await screen.findByText("Submitted")).toBeInTheDocument();
});
```

- [ ] **Step 2: Run the frontend workspace test and confirm RED**

Run:

```bash
cd apps/web && npm test -- annotation-workspace
```

Expected:

```text
FAIL ... Cannot find module '../workspace/api'
FAIL ... No routes matched location "/tasks/18/workspace"
```

- [ ] **Step 3: Implement the image-first workspace shell**

Route and page skeleton:

```tsx
<Route path="/tasks/:taskId/workspace" element={<AnnotationWorkspacePage />} />
```

```tsx
export function AnnotationWorkspacePage() {
  const { taskId = "" } = useParams();
  const workspaceQuery = useQuery({
    queryKey: ["annotation-workspace", taskId],
    queryFn: () => getAnnotationWorkspace(taskId),
  });

  return (
    <section className="workspace-shell">
      <aside className="workspace-tools">
        <button type="button">Add Box</button>
        <button type="button">Clear Draft</button>
      </aside>
      <main className="workspace-canvas">
        <h1>Annotation Workspace</h1>
        <pre>{JSON.stringify(workspaceQuery.data?.draft.body, null, 2)}</pre>
      </main>
      <aside className="workspace-sidebar">
        <button type="button">Save Draft</button>
        <button type="button">Submit Task</button>
      </aside>
    </section>
  );
}
```

Add a launch CTA in `apps/web/src/features/tasks/task-detail-page.tsx`:

```tsx
{task.kind === "annotation" ? (
  <Link to={`/tasks/${task.id}/workspace`} className="button-secondary">
    Open Workspace
  </Link>
) : null}
```

- [ ] **Step 4: Re-run frontend tests and production build**

Run:

```bash
cd apps/web && npm test
cd apps/web && npm run build
```

Expected:

```text
Test Files  ... passed
✓ built in ...
```

- [ ] **Step 5: Commit the web workspace shell**

```bash
git add apps/web/src/features/workspace/api.ts apps/web/src/features/workspace/annotation-workspace-page.tsx apps/web/src/features/workspace/annotation-workspace.test.tsx apps/web/src/app/router.tsx apps/web/src/app/layout/app-shell.tsx apps/web/src/app/styles.css apps/web/src/features/tasks/task-detail-page.tsx apps/web/src/features/tasks/task-detail-page.test.tsx apps/web/src/features/data/api.ts
git commit -m "feat: add annotation workspace shell"
```

## Task 5: Close Out Contract And Smoke Coverage

**Files:**

- Modify: `api/openapi/mvp.yaml`
- Modify: `scripts/dev/smoke.sh`
- Modify: `scripts/dev/smoke_test.go`

- [ ] **Step 1: Add the failing smoke assertions for workspace load, draft save, and submit**

Extend `scripts/dev/smoke_test.go` expected call log with:

```go
for _, fragment := range []string{
	"/v1/tasks",
	"/v1/tasks/1/workspace",
	"/v1/tasks/1/workspace/draft",
	"/v1/tasks/1/workspace/submit",
} {
	if !strings.Contains(callText, fragment) {
		t.Fatalf("expected smoke script to call %s, got log:\n%s", fragment, callText)
	}
}
```

- [ ] **Step 2: Run the smoke-package tests and confirm RED**

Run:

```bash
env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./scripts/dev -count=1 -v
```

Expected:

```text
FAIL ... expected smoke script to call /v1/tasks/1/workspace
```

- [ ] **Step 3: Update the OpenAPI contract and smoke path**

Add these paths to `api/openapi/mvp.yaml`:

```yaml
  /v1/tasks/{id}/workspace:
    get:
      summary: Get annotation workspace payload
  /v1/tasks/{id}/workspace/draft:
    put:
      summary: Save annotation workspace draft
  /v1/tasks/{id}/workspace/submit:
    post:
      summary: Submit annotation workspace task
```

Add this workflow to `scripts/dev/smoke.sh` after the dataset/snapshot setup, starting with an explicit annotation task create:

```bash
task_response="$(curl -fsS -X POST "${api_base}/v1/tasks" \
  -H 'Content-Type: application/json' \
  -d "{\"project_id\":1,\"snapshot_id\":${snapshot_id},\"title\":\"Annotate smoke image\",\"kind\":\"annotation\",\"status\":\"ready\",\"assignee\":\"annotator-1\",\"asset_object_key\":\"train/a.jpg\",\"media_kind\":\"image\",\"ontology_version\":\"v1\"}")" || fail "task create request failed"
task_id="$(json_int_field "$task_response" "id")"
[[ -n "$task_id" ]] || fail "task create response missing id: $task_response"

workspace_response="$(curl -fsS "${api_base}/v1/tasks/${task_id}/workspace")" || fail "workspace request failed"
draft_response="$(curl -fsS -X PUT "${api_base}/v1/tasks/${task_id}/workspace/draft" -H 'Content-Type: application/json' -d '{"actor":"annotator-1","body":{"objects":[{"id":"box-1","label":"person"}]}}')" || fail "workspace draft save failed"
submit_response="$(curl -fsS -X POST "${api_base}/v1/tasks/${task_id}/workspace/submit" -H 'Content-Type: application/json' -d '{"actor":"annotator-1"}')" || fail "workspace submit failed"
```

- [ ] **Step 4: Run the full verification set**

Run:

```bash
env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/annotations ./internal/tasks ./internal/server ./cmd/api-server ./scripts/dev -count=1 -v
cd apps/web && npm test
cd apps/web && npm run build
```

Expected:

```text
PASS
ok  	yolo-ave-mujica/internal/annotations	...
ok  	yolo-ave-mujica/scripts/dev	...
Test Files  ... passed
✓ built in ...
```

- [ ] **Step 5: Commit the delivery closeout**

```bash
git add api/openapi/mvp.yaml scripts/dev/smoke.sh scripts/dev/smoke_test.go
git commit -m "test: extend smoke flow for annotation workspace"
```

## Exit Criteria

This slice is complete when all of the following are true:

1. annotation tasks can carry asset context and reach `submitted`
2. one mutable draft annotation record exists per task
3. the backend serves one aggregated workspace payload plus autosave and submit endpoints
4. the web app exposes `/tasks/:taskId/workspace` and can save and submit a draft
5. OpenAPI and smoke coverage reflect the new flow
6. AI candidate actions and video-only behavior are still explicitly deferred
