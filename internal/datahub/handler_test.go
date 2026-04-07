package datahub_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"yolo-ave-mujica/internal/audit"
	"yolo-ave-mujica/internal/auth"
	"yolo-ave-mujica/internal/datahub"
	"yolo-ave-mujica/internal/jobs"
	"yolo-ave-mujica/internal/observability"
	"yolo-ave-mujica/internal/server"
)

func newTestServerWithFakePresigner() *server.HTTPServer {
	svc := datahub.NewService(func(_ int64, _ string, _ int) (string, error) {
		return "https://signed.local/object", nil
	})
	h := datahub.NewHandler(svc)
	return newTestDataHubServer(h, nil)
}

func newTestDataHubServer(h *datahub.Handler, mutationMiddleware func(http.Handler) http.Handler) *server.HTTPServer {
	return newTestDataHubServerWithMiddlewares(h, nil, mutationMiddleware)
}

func newTestDataHubServerWithMiddlewares(h *datahub.Handler, httpMiddleware, mutationMiddleware func(http.Handler) http.Handler) *server.HTTPServer {
	return server.NewHTTPServerWithModules(server.Modules{
		DataHub: server.DataHubRoutes{
			CreateDataset:          h.CreateDataset,
			ListDatasets:           h.ListDatasets,
			GetDatasetDetail:       h.GetDatasetDetail,
			GetSnapshotDetail:      h.GetSnapshotDetail,
			ScanDataset:            h.ScanDataset,
			CreateSnapshot:         h.CreateSnapshot,
			ListSnapshots:          h.ListSnapshots,
			ListItems:              h.ListItems,
			PresignObject:          h.PresignObject,
			ImportSnapshot:         h.ImportSnapshot,
			CompleteImportSnapshot: h.CompleteImportSnapshot,
		},
		HTTPMiddleware:     httpMiddleware,
		MutationMiddleware: mutationMiddleware,
	})
}

func mustSeedCategory(t *testing.T, repo *datahub.InMemoryRepository, projectID int64, categoryName string) int64 {
	t.Helper()

	categoryID, err := repo.EnsureCategory(context.Background(), projectID, categoryName)
	if err != nil {
		t.Fatalf("seed category %s: %v", categoryName, err)
	}
	return categoryID
}

func TestPresignEndpointReturnsURL(t *testing.T) {
	srv := newTestServerWithFakePresigner()
	createReq := httptest.NewRequest(http.MethodPost, "/v1/datasets", strings.NewReader(`{"project_id":1,"name":"presign-dataset","bucket":"bkt","prefix":"train"}`))
	createRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("create dataset failed: %d body=%s", createRec.Code, createRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/objects/presign", strings.NewReader(`{"dataset_id":1,"object_key":"train/a.jpg"}`))
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "https://") {
		t.Fatalf("expected signed URL, got %s", rec.Body.String())
	}
}

func TestCreateDatasetRejectsProjectOutsideCallerScope(t *testing.T) {
	svc := datahub.NewService(nil)
	h := datahub.NewHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/v1/datasets", strings.NewReader(`{"project_id":2,"name":"forbidden-dataset","bucket":"bkt","prefix":"train"}`))
	req = req.WithContext(auth.WithIdentity(req.Context(), auth.NewIdentity("reviewer-1", []int64{1})))
	rec := httptest.NewRecorder()

	h.CreateDataset(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "project 2") {
		t.Fatalf("expected forbidden project error, got %s", rec.Body.String())
	}
}

func TestPresignObjectRejectsDatasetOutsideCallerScope(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(func(_ int64, _ string, _ int) (string, error) {
		return "https://signed.local/object", nil
	}, repo)
	if _, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 2,
		Name:      "foreign-dataset",
		Bucket:    "platform-dev",
		Prefix:    "foreign/train",
	}); err != nil {
		t.Fatalf("create dataset: %v", err)
	}

	h := datahub.NewHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/v1/objects/presign", strings.NewReader(`{"dataset_id":1,"object_key":"foreign/train/a.jpg"}`))
	req = req.WithContext(auth.WithIdentity(req.Context(), auth.NewIdentity("reviewer-1", []int64{1})))
	rec := httptest.NewRecorder()

	h.PresignObject(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateDatasetAndSnapshotWriteAuditEvents(t *testing.T) {
	recorder := audit.NewRecorder()
	svc := datahub.NewService(func(_ int64, _ string, _ int) (string, error) {
		return "https://signed.local/object", nil
	})
	h := datahub.NewHandlerWithJobsAndSourcePresignAndAudit(svc, nil, nil, recorder)
	srv := server.NewHTTPServerWithDataHub(h)

	createDatasetReq := httptest.NewRequest(http.MethodPost, "/v1/datasets", strings.NewReader(`{"project_id":1,"name":"audit-dataset","bucket":"bkt","prefix":"train"}`))
	createDatasetRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(createDatasetRec, createDatasetReq)
	if createDatasetRec.Code != http.StatusOK {
		t.Fatalf("create dataset failed: %d body=%s", createDatasetRec.Code, createDatasetRec.Body.String())
	}

	createSnapshotReq := httptest.NewRequest(http.MethodPost, "/v1/datasets/1/snapshots", strings.NewReader(`{"note":"audit snapshot"}`))
	createSnapshotRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(createSnapshotRec, createSnapshotReq)
	if createSnapshotRec.Code != http.StatusOK {
		t.Fatalf("create snapshot failed: %d body=%s", createSnapshotRec.Code, createSnapshotRec.Body.String())
	}

	events := recorder.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 audit events, got %+v", events)
	}
	if events[0].Action != "dataset.create" || events[0].ResourceType != "dataset" || events[0].ResourceID != "1" {
		t.Fatalf("unexpected dataset audit event: %+v", events[0])
	}
	if events[1].Action != "snapshot.create" || events[1].ResourceType != "snapshot" || events[1].ResourceID != "1" {
		t.Fatalf("unexpected snapshot audit event: %+v", events[1])
	}
}

func TestScanAndListItems(t *testing.T) {
	srv := newTestServerWithFakePresigner()

	recCreate := httptest.NewRecorder()
	reqCreate := httptest.NewRequest(http.MethodPost, "/v1/datasets", strings.NewReader(`{"project_id":1,"name":"d1","bucket":"bkt","prefix":"train"}`))
	srv.Handler.ServeHTTP(recCreate, reqCreate)
	if recCreate.Code != http.StatusOK {
		t.Fatalf("create dataset failed: %d body=%s", recCreate.Code, recCreate.Body.String())
	}

	recScan := httptest.NewRecorder()
	reqScan := httptest.NewRequest(http.MethodPost, "/v1/datasets/1/scan", strings.NewReader(`{"object_keys":["train/a.jpg"]}`))
	srv.Handler.ServeHTTP(recScan, reqScan)
	if recScan.Code != http.StatusOK {
		t.Fatalf("scan failed: %d body=%s", recScan.Code, recScan.Body.String())
	}

	recItems := httptest.NewRecorder()
	reqItems := httptest.NewRequest(http.MethodGet, "/v1/datasets/1/items", nil)
	srv.Handler.ServeHTTP(recItems, reqItems)
	if recItems.Code != http.StatusOK {
		t.Fatalf("list items failed: %d body=%s", recItems.Code, recItems.Body.String())
	}
	if !strings.Contains(recItems.Body.String(), "train/a.jpg") {
		t.Fatalf("expected indexed object in items list, got %s", recItems.Body.String())
	}
}

func TestListDatasetsIncludesCreatedDatasetSummary(t *testing.T) {
	srv := newTestServerWithFakePresigner()

	recCreate := httptest.NewRecorder()
	reqCreate := httptest.NewRequest(http.MethodPost, "/v1/datasets", strings.NewReader(`{"project_id":1,"name":"browse-d1","bucket":"bkt","prefix":"train"}`))
	srv.Handler.ServeHTTP(recCreate, reqCreate)
	if recCreate.Code != http.StatusOK {
		t.Fatalf("create dataset failed: %d body=%s", recCreate.Code, recCreate.Body.String())
	}

	recList := httptest.NewRecorder()
	reqList := httptest.NewRequest(http.MethodGet, "/v1/datasets", nil)
	srv.Handler.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("list datasets failed: %d body=%s", recList.Code, recList.Body.String())
	}
	if !strings.Contains(recList.Body.String(), `"name":"browse-d1"`) {
		t.Fatalf("expected created dataset in dataset summary list, got %s", recList.Body.String())
	}
}

func TestGetSnapshotDetailIncludesDatasetNameAndVersion(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(nil, repo)
	h := datahub.NewHandler(svc)
	srv := server.NewHTTPServerWithDataHub(h)

	dataset, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 1,
		Name:      "detail-dataset",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	snapshot, err := svc.CreateSnapshot(dataset.ID, datahub.CreateSnapshotInput{Note: "detail target"})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/snapshots/1", nil)
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get snapshot detail failed: %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"dataset_name":"detail-dataset"`) {
		t.Fatalf("expected dataset_name in snapshot detail, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"version":"`+snapshot.Version+`"`) {
		t.Fatalf("expected snapshot version in response, got %s", rec.Body.String())
	}
}

func TestGetDatasetDetailReturnsNotFoundForUnknownDataset(t *testing.T) {
	srv := newTestServerWithFakePresigner()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/datasets/999", nil)
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown dataset detail, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetDatasetDetailReturnsNotFoundOutsideFixedProjectContext(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(nil, repo)
	h := datahub.NewHandler(svc)
	srv := newTestDataHubServerWithMiddlewares(h, auth.IdentityMiddleware([]int64{1}), nil)

	dataset, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 2,
		Name:      "foreign-dataset",
		Bucket:    "platform-dev",
		Prefix:    "foreign/train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/datasets/"+strconv.FormatInt(dataset.ID, 10), nil)
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for out-of-project dataset detail, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestListDatasetsUsesCallerProjectScopes(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(nil, repo)
	if _, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 1,
		Name:      "project-one-dataset",
		Bucket:    "platform-dev",
		Prefix:    "train",
	}); err != nil {
		t.Fatalf("create dataset project 1: %v", err)
	}
	if _, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 2,
		Name:      "project-two-dataset",
		Bucket:    "platform-dev",
		Prefix:    "foreign/train",
	}); err != nil {
		t.Fatalf("create dataset project 2: %v", err)
	}

	h := datahub.NewHandler(svc)
	srv := newTestDataHubServerWithMiddlewares(h, auth.IdentityMiddleware([]int64{2}), nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/datasets", nil)
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "project-one-dataset") {
		t.Fatalf("expected project 1 dataset to be filtered out, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "project-two-dataset") {
		t.Fatalf("expected project 2 dataset to be visible, got %s", rec.Body.String())
	}
}

func TestGetDatasetDetailReturnsVisibleDatasetWithinCallerScope(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 2,
		Name:      "scoped-dataset",
		Bucket:    "platform-dev",
		Prefix:    "foreign/train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}

	h := datahub.NewHandler(svc)
	srv := newTestDataHubServerWithMiddlewares(h, auth.IdentityMiddleware([]int64{2}), nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/datasets/"+strconv.FormatInt(dataset.ID, 10), nil)
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"project_id":2`) {
		t.Fatalf("expected project 2 dataset detail, got %s", rec.Body.String())
	}
}

func TestGetSnapshotDetailRejectsNonNumericID(t *testing.T) {
	srv := newTestServerWithFakePresigner()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/snapshots/not-a-number", nil)
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-numeric snapshot detail id, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetSnapshotDetailReturnsNotFoundOutsideFixedProjectContext(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(nil, repo)
	h := datahub.NewHandler(svc)
	srv := newTestDataHubServerWithMiddlewares(h, auth.IdentityMiddleware([]int64{1}), nil)

	dataset, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 2,
		Name:      "foreign-dataset",
		Bucket:    "platform-dev",
		Prefix:    "foreign/train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	snapshot, err := svc.CreateSnapshot(dataset.ID, datahub.CreateSnapshotInput{Note: "foreign snapshot"})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/snapshots/"+strconv.FormatInt(snapshot.ID, 10), nil)
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for out-of-project snapshot detail, got %d body=%s (snapshot_id=%d)", rec.Code, rec.Body.String(), snapshot.ID)
	}
}

func TestListItemsReturnsNotFoundOutsideCallerScope(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 2,
		Name:      "foreign-items-dataset",
		Bucket:    "platform-dev",
		Prefix:    "foreign/train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if _, err := svc.ScanDataset(dataset.ID, []string{"foreign/train/a.jpg"}); err != nil {
		t.Fatalf("scan dataset: %v", err)
	}

	h := datahub.NewHandler(svc)
	srv := newTestDataHubServerWithMiddlewares(h, auth.IdentityMiddleware([]int64{1}), nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/datasets/"+strconv.FormatInt(dataset.ID, 10)+"/items", nil)
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestImportSnapshotQueuesJobWithResolvedDataset(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 1,
		Name:      "import-dataset",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	snapshot, err := svc.CreateSnapshot(dataset.ID, datahub.CreateSnapshotInput{Note: "import target"})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	jobsSvc := jobs.NewService(jobs.NewInMemoryRepository())
	h := datahub.NewHandlerWithJobs(svc, jobsSvc)
	srv := server.NewHTTPServerWithDataHub(h)

	req := httptest.NewRequest(http.MethodPost, "/v1/snapshots/1/import", strings.NewReader(`{
		"format":"yolo",
		"source_uri":"s3://platform-dev/imports/v1.zip",
		"idempotency_key":"snapshot-import-1",
		"required_resource_type":"cpu",
		"required_capabilities":["importer","yolo"]
	}`))
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"job_id":1`) {
		t.Fatalf("expected job_id in response, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"queued"`) {
		t.Fatalf("expected queued status in response, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"dataset_id":1`) {
		t.Fatalf("expected dataset_id in response, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"snapshot_id":1`) {
		t.Fatalf("expected snapshot_id in response, got %s", rec.Body.String())
	}

	job, ok := jobsSvc.GetJob(1)
	if !ok {
		t.Fatal("expected queued import job to be persisted")
	}
	if job.JobType != "snapshot-import" {
		t.Fatalf("expected snapshot-import job, got %s", job.JobType)
	}
	if job.DatasetID != dataset.ID || job.SnapshotID != snapshot.ID {
		t.Fatalf("expected dataset/snapshot linkage, got %+v", job)
	}
	if job.Payload["format"] != "yolo" {
		t.Fatalf("expected format in payload, got %+v", job.Payload)
	}
	if job.Payload["source_uri"] != "s3://platform-dev/imports/v1.zip" {
		t.Fatalf("expected source_uri in payload, got %+v", job.Payload)
	}
}

func TestImportSnapshotRejectsSnapshotOutsideCallerScope(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 2,
		Name:      "foreign-import-dataset",
		Bucket:    "platform-dev",
		Prefix:    "foreign/train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if _, err := svc.CreateSnapshot(dataset.ID, datahub.CreateSnapshotInput{Note: "foreign snapshot"}); err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	jobsSvc := jobs.NewService(jobs.NewInMemoryRepository())
	h := datahub.NewHandlerWithJobs(svc, jobsSvc)
	srv := newTestDataHubServer(h, auth.IdentityMiddleware([]int64{1}))

	req := httptest.NewRequest(http.MethodPost, "/v1/snapshots/1/import", strings.NewReader(`{
		"format":"yolo",
		"idempotency_key":"snapshot-import-forbidden"
	}`))
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	if _, ok := jobsSvc.GetJob(1); ok {
		t.Fatal("did not expect forbidden import request to create a job")
	}
}

func TestImportSnapshotQueuedJobUsesSnapshotProjectID(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 2,
		Name:      "project-two-import-dataset",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	snapshot, err := svc.CreateSnapshot(dataset.ID, datahub.CreateSnapshotInput{Note: "project two snapshot"})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	jobsSvc := jobs.NewService(jobs.NewInMemoryRepository())
	h := datahub.NewHandlerWithJobs(svc, jobsSvc)
	srv := newTestDataHubServer(h, auth.IdentityMiddleware([]int64{2}))

	req := httptest.NewRequest(http.MethodPost, "/v1/snapshots/"+strconv.FormatInt(snapshot.ID, 10)+"/import", strings.NewReader(`{
		"format":"yolo",
		"idempotency_key":"snapshot-import-project-two"
	}`))
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	job, ok := jobsSvc.GetJob(1)
	if !ok {
		t.Fatal("expected queued import job to be persisted")
	}
	if job.ProjectID != 2 {
		t.Fatalf("expected import job project_id=2, got %+v", job)
	}
}

func TestImportSnapshotPersistsTraceIDFromRequestHeader(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 1,
		Name:      "import-trace-dataset",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if _, err := svc.CreateSnapshot(dataset.ID, datahub.CreateSnapshotInput{Note: "import target"}); err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	jobsSvc := jobs.NewService(jobs.NewInMemoryRepository())
	h := datahub.NewHandlerWithJobs(svc, jobsSvc)
	srv := server.NewHTTPServerWithDataHub(h)

	req := httptest.NewRequest(http.MethodPost, "/v1/snapshots/1/import", strings.NewReader(`{
		"format":"yolo",
		"idempotency_key":"snapshot-import-trace"
	}`))
	req.Header.Set(observability.TraceIDHeader, "trace-import-123")
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	job, ok := jobsSvc.GetJob(1)
	if !ok {
		t.Fatal("expected queued import job to be persisted")
	}
	if got := job.Payload["trace_id"]; got != "trace-import-123" {
		t.Fatalf("expected trace_id in payload, got %#v", got)
	}
}

func TestImportSnapshotRejectsUnsupportedFormatBeforeQueueing(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 1,
		Name:      "unsupported-queue-format",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if _, err := svc.CreateSnapshot(dataset.ID, datahub.CreateSnapshotInput{}); err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	jobsSvc := jobs.NewService(jobs.NewInMemoryRepository())
	h := datahub.NewHandlerWithJobs(svc, jobsSvc)
	srv := server.NewHTTPServerWithDataHub(h)

	req := httptest.NewRequest(http.MethodPost, "/v1/snapshots/1/import", strings.NewReader(`{
		"format":"pascal-voc",
		"idempotency_key":"snapshot-import-invalid"
	}`))
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unsupported format") {
		t.Fatalf("expected unsupported format error, got %s", rec.Body.String())
	}
	if _, ok := jobsSvc.GetJob(1); ok {
		t.Fatal("did not expect invalid import format to be queued")
	}
}

func TestImportSnapshotQueuesInlinePayloadForWorker(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 1,
		Name:      "inline-import-dataset",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	_, err = svc.CreateSnapshot(dataset.ID, datahub.CreateSnapshotInput{Note: "import target"})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	jobsSvc := jobs.NewService(jobs.NewInMemoryRepository())
	h := datahub.NewHandlerWithJobs(svc, jobsSvc)
	srv := server.NewHTTPServerWithDataHub(h)

	req := httptest.NewRequest(http.MethodPost, "/v1/snapshots/1/import", strings.NewReader(`{
		"format":"yolo",
		"source_uri":"s3://platform-dev/imports/v1.zip",
		"idempotency_key":"snapshot-import-inline",
		"labels":{"train/a.txt":"0 0.5 0.5 0.2 0.2\n"},
		"names":["person"],
		"images":{"train/a.txt":"train/a.jpg"}
	}`))
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	job, ok := jobsSvc.GetJob(1)
	if !ok {
		t.Fatal("expected queued import job to be persisted")
	}
	if _, ok := job.Payload["labels"]; !ok {
		t.Fatalf("expected labels in payload, got %+v", job.Payload)
	}
	if _, ok := job.Payload["names"]; !ok {
		t.Fatalf("expected names in payload, got %+v", job.Payload)
	}
	if _, ok := job.Payload["images"]; !ok {
		t.Fatalf("expected images in payload, got %+v", job.Payload)
	}
}

func TestImportSnapshotQueuesSourceDownloadURLWhenPresignerConfigured(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 1,
		Name:      "source-uri-import-dataset",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if _, err := svc.CreateSnapshot(dataset.ID, datahub.CreateSnapshotInput{Note: "import target"}); err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	jobsSvc := jobs.NewService(jobs.NewInMemoryRepository())
	h := datahub.NewHandlerWithJobsAndSourcePresign(svc, jobsSvc, func(sourceURI string, ttlSeconds int) (string, error) {
		return "http://download.local/import.zip", nil
	})
	srv := server.NewHTTPServerWithDataHub(h)

	req := httptest.NewRequest(http.MethodPost, "/v1/snapshots/1/import", strings.NewReader(`{
		"format":"yolo",
		"source_uri":"s3://platform-dev/imports/v1.zip",
		"idempotency_key":"snapshot-import-source-uri"
	}`))
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	job, ok := jobsSvc.GetJob(1)
	if !ok {
		t.Fatal("expected queued import job to be persisted")
	}
	if job.Payload["source_download_url"] != "http://download.local/import.zip" {
		t.Fatalf("expected source_download_url in payload, got %+v", job.Payload)
	}
}

func TestImportSnapshotRejectsOversizedRequestBody(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 1,
		Name:      "oversized-import-dataset",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if _, err := svc.CreateSnapshot(dataset.ID, datahub.CreateSnapshotInput{Note: "import target"}); err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	jobsSvc := jobs.NewService(jobs.NewInMemoryRepository())
	h := datahub.NewHandlerWithJobs(svc, jobsSvc)
	srv := server.NewHTTPServerWithDataHub(h)

	oversizedSourceURI := strings.Repeat("a", 9*1024*1024)
	req := httptest.NewRequest(http.MethodPost, "/v1/snapshots/1/import", strings.NewReader(`{
		"format":"yolo",
		"source_uri":"`+oversizedSourceURI+`",
		"idempotency_key":"snapshot-import-too-large"
	}`))
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "request body exceeds") {
		t.Fatalf("expected oversized request body error, got %s", rec.Body.String())
	}
	if _, ok := jobsSvc.GetJob(1); ok {
		t.Fatal("did not expect oversized import request to create a job")
	}
}

func TestCompleteImportSnapshotPersistsCanonicalAnnotations(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 1,
		Name:      "import-complete-dataset",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if _, err := svc.ScanDataset(dataset.ID, []string{"train/a.jpg"}); err != nil {
		t.Fatalf("scan dataset: %v", err)
	}
	mustSeedCategory(t, repo, dataset.ProjectID, "person")
	if _, err := svc.CreateSnapshot(dataset.ID, datahub.CreateSnapshotInput{Note: "import target"}); err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	h := datahub.NewHandler(svc)
	srv := server.NewHTTPServerWithDataHub(h)

	req := httptest.NewRequest(http.MethodPost, "/internal/snapshots/1/import", strings.NewReader(`{
		"format":"yolo",
		"entries":[
			{
				"object_key":"train/a.jpg",
				"category_name":"person",
				"bbox_x":0.1,
				"bbox_y":0.2,
				"bbox_w":0.3,
				"bbox_h":0.4
			}
		]
	}`))
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"ImportedAnnotations":1`) {
		t.Fatalf("expected imported annotation count in response, got %s", rec.Body.String())
	}
}

func TestCompleteImportSnapshotRejectsOversizedRequestBody(t *testing.T) {
	svc := datahub.NewService(func(_ int64, _ string, _ int) (string, error) {
		return "https://signed.local/object", nil
	})
	h := datahub.NewHandler(svc)
	srv := server.NewHTTPServerWithDataHub(h)

	oversizedSourceURI := strings.Repeat("a", 9*1024*1024)
	req := httptest.NewRequest(http.MethodPost, "/internal/snapshots/1/import", strings.NewReader(`{
		"format":"yolo",
		"source_uri":"`+oversizedSourceURI+`",
		"entries":[]
	}`))
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "request body exceeds") {
		t.Fatalf("expected oversized request body error, got %s", rec.Body.String())
	}
}

func TestCompleteImportSnapshotRejectsUnknownCategory(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 1,
		Name:      "import-unknown-category-dataset",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if _, err := svc.ScanDataset(dataset.ID, []string{"train/a.jpg"}); err != nil {
		t.Fatalf("scan dataset: %v", err)
	}
	if _, err := svc.CreateSnapshot(dataset.ID, datahub.CreateSnapshotInput{Note: "import target"}); err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	h := datahub.NewHandler(svc)
	srv := server.NewHTTPServerWithDataHub(h)

	req := httptest.NewRequest(http.MethodPost, "/internal/snapshots/1/import", strings.NewReader(`{
		"format":"yolo",
		"entries":[
			{
				"object_key":"train/a.jpg",
				"category_name":"person",
				"bbox_x":0.1,
				"bbox_y":0.2,
				"bbox_w":0.3,
				"bbox_h":0.4
			}
		]
	}`))
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unknown category") {
		t.Fatalf("expected unknown category error, got %s", rec.Body.String())
	}
}

func TestCompleteImportSnapshotReturnsNotFoundForUnknownObjectKey(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 1,
		Name:      "import-missing-object-dataset",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if _, err := svc.ScanDataset(dataset.ID, []string{"train/a.jpg"}); err != nil {
		t.Fatalf("scan dataset: %v", err)
	}
	mustSeedCategory(t, repo, dataset.ProjectID, "person")
	if _, err := svc.CreateSnapshot(dataset.ID, datahub.CreateSnapshotInput{Note: "import target"}); err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	h := datahub.NewHandler(svc)
	srv := server.NewHTTPServerWithDataHub(h)

	req := httptest.NewRequest(http.MethodPost, "/internal/snapshots/1/import", strings.NewReader(`{
		"format":"yolo",
		"entries":[
			{
				"object_key":"train/missing.jpg",
				"category_name":"person",
				"bbox_x":0.1,
				"bbox_y":0.2,
				"bbox_w":0.3,
				"bbox_h":0.4
			}
		]
	}`))
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "not found") {
		t.Fatalf("expected not found error, got %s", rec.Body.String())
	}
}

func TestCompleteImportSnapshotIsIdempotentForExactReplay(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 1,
		Name:      "import-idempotent-handler-dataset",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if _, err := svc.ScanDataset(dataset.ID, []string{"train/a.jpg"}); err != nil {
		t.Fatalf("scan dataset: %v", err)
	}
	mustSeedCategory(t, repo, dataset.ProjectID, "person")
	if _, err := svc.CreateSnapshot(dataset.ID, datahub.CreateSnapshotInput{Note: "import target"}); err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	h := datahub.NewHandler(svc)
	srv := server.NewHTTPServerWithDataHub(h)

	body := `{
		"format":"yolo",
		"entries":[
			{
				"object_key":"train/a.jpg",
				"category_name":"person",
				"bbox_x":0.1,
				"bbox_y":0.2,
				"bbox_w":0.3,
				"bbox_h":0.4
			}
		]
	}`

	firstReq := httptest.NewRequest(http.MethodPost, "/internal/snapshots/1/import", strings.NewReader(body))
	firstRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected first import 200, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	retryReq := httptest.NewRequest(http.MethodPost, "/internal/snapshots/1/import", strings.NewReader(body))
	retryRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(retryRec, retryReq)
	if retryRec.Code != http.StatusOK {
		t.Fatalf("expected idempotent replay 200, got %d body=%s", retryRec.Code, retryRec.Body.String())
	}

	if len(repo.AnnotationsForSnapshot(1)) != 1 {
		t.Fatalf("expected idempotent replay to keep one stored annotation, got %+v", repo.AnnotationsForSnapshot(1))
	}
}

func TestCompleteImportSnapshotReturnsConflictForDifferentReplay(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(nil, repo)

	dataset, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 1,
		Name:      "import-conflict-handler-dataset",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if _, err := svc.ScanDataset(dataset.ID, []string{"train/a.jpg", "train/b.jpg"}); err != nil {
		t.Fatalf("scan dataset: %v", err)
	}
	mustSeedCategory(t, repo, dataset.ProjectID, "person")
	if _, err := svc.CreateSnapshot(dataset.ID, datahub.CreateSnapshotInput{Note: "import target"}); err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	h := datahub.NewHandler(svc)
	srv := server.NewHTTPServerWithDataHub(h)

	firstReq := httptest.NewRequest(http.MethodPost, "/internal/snapshots/1/import", strings.NewReader(`{
		"format":"yolo",
		"entries":[
			{
				"object_key":"train/a.jpg",
				"category_name":"person",
				"bbox_x":0.1,
				"bbox_y":0.2,
				"bbox_w":0.3,
				"bbox_h":0.4
			}
		]
	}`))
	firstRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected first import 200, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	conflictReq := httptest.NewRequest(http.MethodPost, "/internal/snapshots/1/import", strings.NewReader(`{
		"format":"yolo",
		"entries":[
			{
				"object_key":"train/b.jpg",
				"category_name":"person",
				"bbox_x":0.11,
				"bbox_y":0.22,
				"bbox_w":0.33,
				"bbox_h":0.44
			}
		]
	}`))
	conflictRec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(conflictRec, conflictReq)

	if conflictRec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", conflictRec.Code, conflictRec.Body.String())
	}
	if !strings.Contains(strings.ToLower(conflictRec.Body.String()), "already completed") {
		t.Fatalf("expected conflict detail, got %s", conflictRec.Body.String())
	}
}

func TestCreateSnapshotRejectsProjectOutsideCallerScope(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(nil, repo)
	dataset, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 1,
		Name:      "foreign-snapshot-dataset",
		Bucket:    "platform-dev",
		Prefix:    "foreign/train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}

	h := datahub.NewHandler(svc)
	srv := newTestDataHubServer(h, auth.IdentityMiddleware([]int64{2}))

	req := httptest.NewRequest(http.MethodPost, "/v1/datasets/"+strconv.FormatInt(dataset.ID, 10)+"/snapshots", strings.NewReader(`{"note":"attempted create snapshot"}`))
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for snapshot create, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestScanDatasetRejectsProjectOutsideCallerScope(t *testing.T) {
	repo := datahub.NewInMemoryRepository()
	svc := datahub.NewServiceWithRepository(nil, repo)
	dataset, err := svc.CreateDataset(datahub.CreateDatasetInput{
		ProjectID: 1,
		Name:      "foreign-scan-dataset",
		Bucket:    "platform-dev",
		Prefix:    "fore/scan",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}

	h := datahub.NewHandler(svc)
	srv := newTestDataHubServer(h, auth.IdentityMiddleware([]int64{2}))

	req := httptest.NewRequest(http.MethodPost, "/v1/datasets/"+strconv.FormatInt(dataset.ID, 10)+"/scan", strings.NewReader(`{"object_keys":["a.jpg"]}`))
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for dataset scan, got %d body=%s", rec.Code, rec.Body.String())
	}
}
