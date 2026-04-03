package datahub_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"yolo-ave-mujica/internal/datahub"
	"yolo-ave-mujica/internal/jobs"
	"yolo-ave-mujica/internal/server"
)

func newTestServerWithFakePresigner() *server.HTTPServer {
	svc := datahub.NewService(func(_ int64, _ string, _ int) (string, error) {
		return "https://signed.local/object", nil
	})
	h := datahub.NewHandler(svc)
	return server.NewHTTPServerWithDataHub(h)
}

func TestPresignEndpointReturnsURL(t *testing.T) {
	srv := newTestServerWithFakePresigner()
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
	srv := server.NewHTTPServerWithDataHub(h)

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
	srv := server.NewHTTPServerWithDataHub(h)

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
