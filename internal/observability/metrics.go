package observability

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type httpMetricKey struct {
	Method      string
	Route       string
	StatusClass string
}

type httpDurationKey struct {
	Method string
	Route  string
}

type durationAggregate struct {
	SumSeconds float64
	Count      int64
}

type jobMetricKey struct {
	JobType string
	Status  string
}

type Metrics struct {
	mu                    sync.Mutex
	httpRequests          map[httpMetricKey]int64
	httpDurations         map[httpDurationKey]durationAggregate
	jobCreations          map[string]int64
	jobCompletions        map[jobMetricKey]int64
	jobDurations          map[jobMetricKey]durationAggregate
	leaseRecoveries       map[string]int64
	artifactBuildOutcomes map[string]int64
	queueDepthProvider    func(context.Context) (map[string]int64, error)
	reviewBacklogProvider func(context.Context) (int64, error)
}

func NewMetrics() *Metrics {
	return &Metrics{
		httpRequests:          make(map[httpMetricKey]int64),
		httpDurations:         make(map[httpDurationKey]durationAggregate),
		jobCreations:          make(map[string]int64),
		jobCompletions:        make(map[jobMetricKey]int64),
		jobDurations:          make(map[jobMetricKey]durationAggregate),
		leaseRecoveries:       make(map[string]int64),
		artifactBuildOutcomes: make(map[string]int64),
	}
}

func (m *Metrics) ObserveHTTPRequest(route, method string, status int, duration time.Duration) {
	if m == nil {
		return
	}
	route = strings.TrimSpace(route)
	if route == "" {
		route = "/"
	}
	method = strings.TrimSpace(method)
	if method == "" {
		method = http.MethodGet
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	requestKey := httpMetricKey{
		Method:      method,
		Route:       route,
		StatusClass: statusClass(status),
	}
	m.httpRequests[requestKey]++

	durationKey := httpDurationKey{
		Method: method,
		Route:  route,
	}
	aggregate := m.httpDurations[durationKey]
	aggregate.Count++
	aggregate.SumSeconds += duration.Seconds()
	m.httpDurations[durationKey] = aggregate
}

func (m *Metrics) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(m.render(r.Context())))
	}
}

func (m *Metrics) IncJobCreated(jobType string) {
	if m == nil {
		return
	}
	jobType = strings.TrimSpace(jobType)
	if jobType == "" {
		jobType = "unknown"
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobCreations[jobType]++
}

func (m *Metrics) ObserveJobCompleted(jobType, status string, duration time.Duration) {
	if m == nil {
		return
	}
	jobType = strings.TrimSpace(jobType)
	if jobType == "" {
		jobType = "unknown"
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = "unknown"
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	key := jobMetricKey{JobType: jobType, Status: status}
	m.jobCompletions[key]++
	aggregate := m.jobDurations[key]
	aggregate.Count++
	aggregate.SumSeconds += duration.Seconds()
	m.jobDurations[key] = aggregate
}

func (m *Metrics) IncLeaseRecovery(jobType string) {
	if m == nil {
		return
	}
	jobType = strings.TrimSpace(jobType)
	if jobType == "" {
		jobType = "unknown"
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.leaseRecoveries[jobType]++
}

func (m *Metrics) IncArtifactBuildOutcome(status string) {
	if m == nil {
		return
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = "unknown"
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.artifactBuildOutcomes[status]++
}

func (m *Metrics) SetQueueDepthProvider(provider func(context.Context) (map[string]int64, error)) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queueDepthProvider = provider
}

func (m *Metrics) SetReviewBacklogProvider(provider func(context.Context) (int64, error)) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reviewBacklogProvider = provider
}

func (m *Metrics) render(ctx context.Context) string {
	if m == nil {
		return ""
	}

	m.mu.Lock()
	requests := make(map[httpMetricKey]int64, len(m.httpRequests))
	for key, value := range m.httpRequests {
		requests[key] = value
	}
	durations := make(map[httpDurationKey]durationAggregate, len(m.httpDurations))
	for key, value := range m.httpDurations {
		durations[key] = value
	}
	jobCreations := make(map[string]int64, len(m.jobCreations))
	for key, value := range m.jobCreations {
		jobCreations[key] = value
	}
	jobCompletions := make(map[jobMetricKey]int64, len(m.jobCompletions))
	for key, value := range m.jobCompletions {
		jobCompletions[key] = value
	}
	jobDurations := make(map[jobMetricKey]durationAggregate, len(m.jobDurations))
	for key, value := range m.jobDurations {
		jobDurations[key] = value
	}
	leaseRecoveries := make(map[string]int64, len(m.leaseRecoveries))
	for key, value := range m.leaseRecoveries {
		leaseRecoveries[key] = value
	}
	artifactBuildOutcomes := make(map[string]int64, len(m.artifactBuildOutcomes))
	for key, value := range m.artifactBuildOutcomes {
		artifactBuildOutcomes[key] = value
	}
	queueDepthProvider := m.queueDepthProvider
	reviewBacklogProvider := m.reviewBacklogProvider
	m.mu.Unlock()

	var lines []string
	lines = append(lines,
		"# TYPE yolo_http_requests_total counter",
	)

	requestKeys := make([]httpMetricKey, 0, len(requests))
	for key := range requests {
		requestKeys = append(requestKeys, key)
	}
	sort.Slice(requestKeys, func(i, j int) bool {
		if requestKeys[i].Route != requestKeys[j].Route {
			return requestKeys[i].Route < requestKeys[j].Route
		}
		if requestKeys[i].Method != requestKeys[j].Method {
			return requestKeys[i].Method < requestKeys[j].Method
		}
		return requestKeys[i].StatusClass < requestKeys[j].StatusClass
	})
	for _, key := range requestKeys {
		lines = append(lines, fmt.Sprintf(
			`yolo_http_requests_total{method=%q,route=%q,status_class=%q} %d`,
			key.Method,
			key.Route,
			key.StatusClass,
			requests[key],
		))
	}

	lines = append(lines,
		"# TYPE yolo_http_request_duration_seconds summary",
	)
	durationKeys := make([]httpDurationKey, 0, len(durations))
	for key := range durations {
		durationKeys = append(durationKeys, key)
	}
	sort.Slice(durationKeys, func(i, j int) bool {
		if durationKeys[i].Route != durationKeys[j].Route {
			return durationKeys[i].Route < durationKeys[j].Route
		}
		return durationKeys[i].Method < durationKeys[j].Method
	})
	for _, key := range durationKeys {
		aggregate := durations[key]
		lines = append(lines, fmt.Sprintf(
			`yolo_http_request_duration_seconds_sum{method=%q,route=%q} %.6f`,
			key.Method,
			key.Route,
			aggregate.SumSeconds,
		))
		lines = append(lines, fmt.Sprintf(
			`yolo_http_request_duration_seconds_count{method=%q,route=%q} %d`,
			key.Method,
			key.Route,
			aggregate.Count,
		))
	}

	lines = append(lines,
		"# TYPE yolo_job_creations_total counter",
	)
	jobCreationKeys := make([]string, 0, len(jobCreations))
	for key := range jobCreations {
		jobCreationKeys = append(jobCreationKeys, key)
	}
	sort.Strings(jobCreationKeys)
	for _, key := range jobCreationKeys {
		lines = append(lines, fmt.Sprintf(`yolo_job_creations_total{job_type=%q} %d`, key, jobCreations[key]))
	}

	lines = append(lines,
		"# TYPE yolo_job_completions_total counter",
	)
	jobKeys := make([]jobMetricKey, 0, len(jobCompletions))
	for key := range jobCompletions {
		jobKeys = append(jobKeys, key)
	}
	sort.Slice(jobKeys, func(i, j int) bool {
		if jobKeys[i].JobType != jobKeys[j].JobType {
			return jobKeys[i].JobType < jobKeys[j].JobType
		}
		return jobKeys[i].Status < jobKeys[j].Status
	})
	for _, key := range jobKeys {
		lines = append(lines, fmt.Sprintf(
			`yolo_job_completions_total{job_type=%q,status=%q} %d`,
			key.JobType,
			key.Status,
			jobCompletions[key],
		))
	}

	lines = append(lines,
		"# TYPE yolo_job_duration_seconds summary",
	)
	jobDurationKeys := make([]jobMetricKey, 0, len(jobDurations))
	for key := range jobDurations {
		jobDurationKeys = append(jobDurationKeys, key)
	}
	sort.Slice(jobDurationKeys, func(i, j int) bool {
		if jobDurationKeys[i].JobType != jobDurationKeys[j].JobType {
			return jobDurationKeys[i].JobType < jobDurationKeys[j].JobType
		}
		return jobDurationKeys[i].Status < jobDurationKeys[j].Status
	})
	for _, key := range jobDurationKeys {
		aggregate := jobDurations[key]
		lines = append(lines, fmt.Sprintf(
			`yolo_job_duration_seconds_sum{job_type=%q,status=%q} %.6f`,
			key.JobType,
			key.Status,
			aggregate.SumSeconds,
		))
		lines = append(lines, fmt.Sprintf(
			`yolo_job_duration_seconds_count{job_type=%q,status=%q} %d`,
			key.JobType,
			key.Status,
			aggregate.Count,
		))
	}

	lines = append(lines,
		"# TYPE yolo_job_lease_recoveries_total counter",
	)
	leaseKeys := make([]string, 0, len(leaseRecoveries))
	for key := range leaseRecoveries {
		leaseKeys = append(leaseKeys, key)
	}
	sort.Strings(leaseKeys)
	for _, key := range leaseKeys {
		lines = append(lines, fmt.Sprintf(`yolo_job_lease_recoveries_total{job_type=%q} %d`, key, leaseRecoveries[key]))
	}

	lines = append(lines,
		"# TYPE yolo_artifact_build_outcomes_total counter",
	)
	artifactKeys := make([]string, 0, len(artifactBuildOutcomes))
	for key := range artifactBuildOutcomes {
		artifactKeys = append(artifactKeys, key)
	}
	sort.Strings(artifactKeys)
	for _, key := range artifactKeys {
		lines = append(lines, fmt.Sprintf(`yolo_artifact_build_outcomes_total{status=%q} %d`, key, artifactBuildOutcomes[key]))
	}

	if queueDepthProvider != nil {
		if depths, err := queueDepthProvider(ctx); err == nil {
			lines = append(lines, "# TYPE yolo_queue_depth gauge")
			lanes := make([]string, 0, len(depths))
			for lane := range depths {
				lanes = append(lanes, lane)
			}
			sort.Strings(lanes)
			for _, lane := range lanes {
				lines = append(lines, fmt.Sprintf(`yolo_queue_depth{lane=%q} %d`, lane, depths[lane]))
			}
		}
	}

	if reviewBacklogProvider != nil {
		if backlog, err := reviewBacklogProvider(ctx); err == nil {
			lines = append(lines,
				"# TYPE yolo_review_backlog gauge",
				fmt.Sprintf("yolo_review_backlog %d", backlog),
			)
		}
	}

	return strings.Join(lines, "\n") + "\n"
}

func statusClass(status int) string {
	switch {
	case status >= 100 && status < 200:
		return "1xx"
	case status >= 200 && status < 300:
		return "2xx"
	case status >= 300 && status < 400:
		return "3xx"
	case status >= 400 && status < 500:
		return "4xx"
	case status >= 500 && status < 600:
		return "5xx"
	default:
		return "unknown"
	}
}
