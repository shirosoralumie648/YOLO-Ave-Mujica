package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsHandlerIncludesRuntimeCountersAndDynamicGauges(t *testing.T) {
	metrics := NewMetrics()
	metrics.IncJobCreated("zero-shot")
	metrics.ObserveJobCompleted("zero-shot", "succeeded", 2*time.Second)
	metrics.IncLeaseRecovery("zero-shot")
	metrics.IncArtifactBuildOutcome("ready")
	metrics.SetQueueDepthProvider(func(context.Context) (map[string]int64, error) {
		return map[string]int64{
			"jobs:cpu": 2,
			"jobs:gpu": 1,
		}, nil
	})
	metrics.SetReviewBacklogProvider(func(context.Context) (int64, error) {
		return 4, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec, req)

	body := rec.Body.String()
	for _, needle := range []string{
		`yolo_job_creations_total{job_type="zero-shot"} 1`,
		`yolo_job_completions_total{job_type="zero-shot",status="succeeded"} 1`,
		`yolo_job_lease_recoveries_total{job_type="zero-shot"} 1`,
		`yolo_artifact_build_outcomes_total{status="ready"} 1`,
		`yolo_queue_depth{lane="jobs:cpu"} 2`,
		`yolo_review_backlog 4`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected metrics output to contain %q, got:\n%s", needle, body)
		}
	}
}
