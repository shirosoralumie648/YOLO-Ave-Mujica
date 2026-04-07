package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"yolo-ave-mujica/internal/observability"
)

type Event struct {
	ID         int64          `json:"id"`
	JobID      int64          `json:"job_id"`
	ItemID     *int64         `json:"item_id,omitempty"`
	EventLevel string         `json:"event_level"`
	EventType  string         `json:"event_type"`
	Message    string         `json:"message"`
	Detail     map[string]any `json:"detail_json,omitempty"`
	TS         time.Time      `json:"ts"`
}

type Service struct {
	repo       Repository
	dispatcher Publisher
	reviewSink reviewCandidateSink
	metrics    *observability.Metrics
}

type CandidateBBox struct {
	X float64
	Y float64
	W float64
	H float64
}

type ReviewCandidateInput struct {
	DatasetID    int64
	SnapshotID   int64
	ItemID       int64
	ObjectKey    string
	CategoryID   int64
	CategoryName string
	BBox         CandidateBBox
	Confidence   *float64
	ModelName    string
	IsPseudo     bool
}

type PersistedReviewCandidate struct {
	ID int64
}

type reviewCandidateSink interface {
	PersistCandidates(jobID int64, items []ReviewCandidateInput) ([]PersistedReviewCandidate, error)
}

var defaultRequiredCapabilities = map[string][]string{
	"artifact-package": {"artifact_packaging"},
	"cleaning":         {"rules_engine", "image_stats"},
	"snapshot-import":  {"snapshot_import"},
	"video-extract":    {"video_decode"},
	"zero-shot":        {"zero_shot_inference"},
}

var allowedResourceTypesByJobType = map[string]map[string]bool{
	"artifact-package": {"cpu": true},
	"cleaning":         {"cpu": true, "mixed": true},
	"snapshot-import":  {"cpu": true},
	"video-extract":    {"cpu": true, "mixed": true},
	"zero-shot":        {"gpu": true, "mixed": true},
}

func NewService(repo Repository) *Service {
	return NewServiceWithDependenciesAndMetrics(repo, nil, nil, nil)
}

func NewServiceWithPublisher(repo Repository, dispatcher Publisher) *Service {
	return NewServiceWithDependenciesAndMetrics(repo, dispatcher, nil, nil)
}

func NewServiceWithReviewSink(repo Repository, dispatcher Publisher, reviewSink reviewCandidateSink) *Service {
	return NewServiceWithDependenciesAndMetrics(repo, dispatcher, reviewSink, nil)
}

func NewServiceWithDependencies(repo Repository, dispatcher Publisher, reviewSink reviewCandidateSink) *Service {
	return NewServiceWithDependenciesAndMetrics(repo, dispatcher, reviewSink, nil)
}

func NewServiceWithDependenciesAndMetrics(repo Repository, dispatcher Publisher, reviewSink reviewCandidateSink, metrics *observability.Metrics) *Service {
	if repo == nil {
		repo = NewInMemoryRepository()
	}
	return &Service{repo: repo, dispatcher: dispatcher, reviewSink: reviewSink, metrics: metrics}
}

func (s *Service) CreateJob(in CreateJobInput) (*Job, error) {
	if in.ProjectID <= 0 {
		in.ProjectID = 1
	}
	in.JobType = strings.TrimSpace(strings.ToLower(in.JobType))
	if in.JobType == "" {
		return nil, errors.New("job_type is required")
	}
	if in.IdempotencyKey == "" {
		return nil, errors.New("idempotency_key is required")
	}

	requiredResourceType, err := normalizeRequiredResourceType(in.JobType, in.RequiredResourceType)
	if err != nil {
		return nil, err
	}
	requiredCapabilities, err := normalizeRequiredCapabilities(in.JobType, in.RequiredCapabilities)
	if err != nil {
		return nil, err
	}
	in.RequiredResourceType = requiredResourceType
	in.RequiredCapabilities = requiredCapabilities
	if in.Payload == nil {
		in.Payload = map[string]any{}
	}

	job, created, err := s.repo.CreateOrGet(in)
	if err != nil {
		return nil, err
	}
	if created {
		if s.metrics != nil {
			s.metrics.IncJobCreated(job.JobType)
		}
		if _, err := s.repo.AppendEvent(job.ID, nil, "info", "dispatch_requested", "job queued for dispatch", map[string]any{
			"job_type":               job.JobType,
			"required_resource_type": job.RequiredResourceType,
			"required_capabilities":  job.RequiredCapabilities,
			"resource_lane":          laneFor(job.RequiredResourceType),
		}); err != nil {
			return nil, err
		}
		if s.dispatcher != nil {
			if err := s.dispatcher.Publish(context.Background(), laneFor(job.RequiredResourceType), buildDispatchPayload(job)); err != nil {
				return nil, err
			}
		}
	}
	return job, nil
}

func normalizeRequiredResourceType(jobType, resourceType string) (string, error) {
	resourceType = strings.TrimSpace(strings.ToLower(resourceType))
	if resourceType == "" {
		resourceType = defaultRequiredResourceType(jobType)
	}

	switch resourceType {
	case "cpu", "gpu", "mixed":
	default:
		return "", fmt.Errorf("unsupported required_resource_type %q", resourceType)
	}

	allowed := allowedResourceTypesByJobType[jobType]
	if len(allowed) > 0 && !allowed[resourceType] {
		return "", fmt.Errorf("job_type %q does not support required_resource_type %q", jobType, resourceType)
	}
	return resourceType, nil
}

func defaultRequiredResourceType(jobType string) string {
	switch jobType {
	case "zero-shot":
		return "gpu"
	default:
		return "cpu"
	}
}

func normalizeRequiredCapabilities(jobType string, raw []string) ([]string, error) {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, capability := range raw {
		capability = strings.TrimSpace(strings.ToLower(capability))
		if capability == "" {
			continue
		}
		if _, ok := seen[capability]; ok {
			continue
		}
		seen[capability] = struct{}{}
		out = append(out, capability)
	}

	if len(out) == 0 {
		out = append(out, defaultRequiredCapabilities[jobType]...)
	}
	if len(out) == 0 {
		return nil, errors.New("required_capabilities must not be empty")
	}
	return out, nil
}

func (s *Service) GetJob(id int64) (*Job, bool) {
	job, ok := s.repo.Get(id)
	if !ok {
		return nil, false
	}
	if len(job.ResultRef) > 0 {
		job.ResultType = stringValue(job.ResultRef["result_type"])
		job.ResultCount = int(int64Value(job.ResultRef["result_count"]))
		return job, true
	}
	events, err := s.repo.ListEvents(id)
	if err != nil || len(events) == 0 {
		return job, true
	}
	if resultRef := extractResultRef(events); len(resultRef) > 0 {
		job.ResultRef = resultRef
		job.ResultType = stringValue(resultRef["result_type"])
		job.ResultCount = int(int64Value(resultRef["result_count"]))
	}
	return job, true
}

func (s *Service) RegisterWorker(in RegisterWorkerInput) (*Worker, error) {
	workerID := strings.TrimSpace(in.WorkerID)
	if workerID == "" {
		return nil, errors.New("worker_id is required")
	}

	resourceLane := strings.TrimSpace(strings.ToLower(in.ResourceLane))
	switch resourceLane {
	case "jobs:cpu", "jobs:gpu", "jobs:mixed":
	default:
		return nil, fmt.Errorf("resource_lane must be one of jobs:cpu, jobs:gpu, jobs:mixed")
	}

	jobTypes := normalizeWorkerStrings(in.JobTypes)
	if len(jobTypes) == 0 {
		return nil, errors.New("job_types must not be empty")
	}
	capabilities := normalizeWorkerStrings(in.Capabilities)

	return s.repo.UpsertWorker(RegisterWorkerInput{
		WorkerID:     workerID,
		ResourceLane: resourceLane,
		Capabilities: capabilities,
		JobTypes:     jobTypes,
	})
}

func (s *Service) ListWorkers() ([]Worker, error) {
	return s.repo.ListWorkers()
}

func (s *Service) AppendEvent(jobID int64, itemID *int64, level, typ, message string, detail map[string]any) Event {
	ev, _ := s.repo.AppendEvent(jobID, itemID, level, typ, message, detail)
	return ev
}

func (s *Service) ListEvents(jobID int64) []Event {
	out, _ := s.repo.ListEvents(jobID)
	return out
}

func normalizeWorkerStrings(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(strings.ToLower(item))
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func (s *Service) ReportHeartbeat(jobID int64, workerID string, leaseSeconds int) error {
	if workerID == "" {
		return newValidationError("worker_id is required")
	}
	if leaseSeconds <= 0 {
		leaseSeconds = 30
	}

	leaseUntil := time.Now().UTC().Add(time.Duration(leaseSeconds) * time.Second)
	job, ok := s.repo.Get(jobID)
	if !ok {
		return newNotFoundError("job %d not found", jobID)
	}

	if job.Status == StatusRunning {
		if err := s.repo.TouchLease(jobID, workerID, leaseUntil); err != nil {
			return err
		}
	} else {
		if _, err := s.repo.Claim(jobID, workerID, leaseUntil); err != nil {
			return err
		}
	}

	_, err := s.repo.AppendEvent(jobID, nil, "info", "heartbeat", "worker heartbeat", map[string]any{
		"worker_id":     workerID,
		"lease_seconds": leaseSeconds,
	})
	return err
}

func (s *Service) ReportProgress(jobID int64, workerID string, total, succeeded, failed int) error {
	if workerID == "" {
		return newValidationError("worker_id is required")
	}
	if total < 0 || succeeded < 0 || failed < 0 {
		return newValidationError("progress counters must be >= 0")
	}
	if err := s.repo.UpdateProgress(jobID, workerID, total, succeeded, failed); err != nil {
		return err
	}
	_, err := s.repo.AppendEvent(jobID, nil, "info", "progress", "worker progress", map[string]any{
		"worker_id":       workerID,
		"total_items":     total,
		"succeeded_items": succeeded,
		"failed_items":    failed,
	})
	return err
}

func (s *Service) ReportItemError(jobID, itemID int64, message string, detail map[string]any) error {
	return s.ReportEvent(jobID, &itemID, "error", "item_failed", message, detail)
}

func (s *Service) ReportEvent(jobID int64, itemID *int64, level, typ, message string, detail map[string]any) error {
	if itemID != nil && *itemID <= 0 {
		return newValidationError("item_id must be > 0")
	}
	if strings.TrimSpace(typ) == "" {
		return newValidationError("event_type is required")
	}
	if strings.TrimSpace(message) == "" {
		return newValidationError("message is required")
	}
	if strings.TrimSpace(level) == "" {
		level = "info"
	}
	if itemID == nil && typ == "item_failed" {
		return newValidationError("item_id must be > 0")
	}
	detail, err := s.normalizeEventDetail(jobID, typ, detail)
	if err != nil {
		return err
	}
	if _, err := s.repo.AppendEvent(jobID, itemID, level, typ, message, detail); err != nil {
		return err
	}
	if stringValue(detail["result_type"]) != "" {
		return s.repo.StoreResultRef(jobID, normalizeResultRef(detail))
	}
	return nil
}

func (s *Service) ReportTerminal(jobID int64, workerID, status string, total, succeeded, failed int, resultRef map[string]any) error {
	if workerID == "" {
		return newValidationError("worker_id is required")
	}
	switch status {
	case StatusSucceeded, StatusSucceededWithErrors, StatusFailed, StatusCanceled:
	default:
		return newValidationError("unsupported terminal status %q", status)
	}
	if total < 0 || succeeded < 0 || failed < 0 {
		return newValidationError("terminal counters must be >= 0")
	}
	job, ok := s.repo.Get(jobID)
	if !ok {
		return newNotFoundError("job %d not found", jobID)
	}
	if err := s.repo.Complete(jobID, workerID, status, total, succeeded, failed); err != nil {
		return err
	}
	detail := map[string]any{
		"worker_id":       workerID,
		"status":          status,
		"total_items":     total,
		"succeeded_items": succeeded,
		"failed_items":    failed,
	}
	for key, value := range normalizeResultRef(resultRef) {
		detail[key] = value
	}
	if _, err := s.repo.AppendEvent(jobID, nil, "info", "terminal", "job completed", detail); err != nil {
		return err
	}
	if err := s.repo.StoreResultRef(jobID, normalizeResultRef(resultRef)); err != nil {
		return err
	}
	if s.metrics != nil {
		s.metrics.ObserveJobCompleted(job.JobType, status, time.Since(job.CreatedAt))
	}
	return nil
}

func (s *Service) normalizeEventDetail(jobID int64, typ string, detail map[string]any) (map[string]any, error) {
	if detail == nil {
		detail = map[string]any{}
	}
	switch typ {
	case "review_candidates_materialized":
		return s.persistReviewCandidates(jobID, detail)
	case "video_frames_materialized":
		return normalizeResultDetail(detail, "video_frames", "frames"), nil
	case "cleaning_report_materialized":
		return normalizeResultDetail(detail, "cleaning_report", "removal_candidates"), nil
	default:
		return detail, nil
	}
}

func (s *Service) persistReviewCandidates(jobID int64, detail map[string]any) (map[string]any, error) {
	if s.reviewSink == nil {
		return nil, errors.New("review sink is not configured")
	}

	rawCandidates, ok := detail["candidates"]
	if !ok {
		return nil, errors.New("candidates are required")
	}

	payload, err := json.Marshal(rawCandidates)
	if err != nil {
		return nil, err
	}

	type candidatePayload struct {
		DatasetID    int64    `json:"dataset_id"`
		SnapshotID   int64    `json:"snapshot_id"`
		ItemID       int64    `json:"item_id"`
		ObjectKey    string   `json:"object_key"`
		CategoryID   int64    `json:"category_id"`
		CategoryName string   `json:"category_name"`
		Confidence   *float64 `json:"confidence"`
		ModelName    string   `json:"model_name"`
		IsPseudo     bool     `json:"is_pseudo"`
		BBox         struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
			W float64 `json:"w"`
			H float64 `json:"h"`
		} `json:"bbox"`
	}

	var raw []candidatePayload
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, errors.New("candidates are required")
	}

	inputs := make([]ReviewCandidateInput, 0, len(raw))
	for _, item := range raw {
		inputs = append(inputs, ReviewCandidateInput{
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
			IsPseudo:   true,
		})
	}

	persisted, err := s.reviewSink.PersistCandidates(jobID, inputs)
	if err != nil {
		return nil, err
	}

	candidateIDs := make([]int64, 0, len(persisted))
	for _, candidate := range persisted {
		candidateIDs = append(candidateIDs, candidate.ID)
	}
	return map[string]any{
		"result_type":   "annotation_candidates",
		"result_count":  len(persisted),
		"candidate_ids": candidateIDs,
	}, nil
}

func normalizeResultDetail(detail map[string]any, defaultType, collectionKey string) map[string]any {
	out := make(map[string]any, len(detail)+2)
	for key, value := range detail {
		out[key] = value
	}
	if stringValue(out["result_type"]) == "" {
		out["result_type"] = defaultType
	}
	if int64Value(out["result_count"]) <= 0 {
		if items, ok := out[collectionKey].([]any); ok {
			out["result_count"] = len(items)
		}
	}
	return out
}

func normalizeResultRef(resultRef map[string]any) map[string]any {
	if len(resultRef) == 0 {
		return nil
	}
	return normalizeResultDetail(resultRef, stringValue(resultRef["result_type"]), "")
}

func extractResultRef(events []Event) map[string]any {
	var out map[string]any
	var resultType string
	for idx := len(events) - 1; idx >= 0; idx-- {
		detail := events[idx].Detail
		if len(detail) == 0 {
			continue
		}
		currentType := stringValue(detail["result_type"])
		if currentType == "" {
			continue
		}
		if out == nil {
			resultType = currentType
			out = map[string]any{
				"result_type": currentType,
			}
		}
		if currentType != resultType {
			continue
		}
		if _, ok := out["result_count"]; !ok {
			if count := int64Value(detail["result_count"]); count > 0 {
				out["result_count"] = count
			}
		}
		for key, value := range detail {
			if key == "result_type" || key == "result_count" || shouldSkipResultRefKey(key) {
				continue
			}
			if _, ok := out[key]; ok {
				continue
			}
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func shouldSkipResultRefKey(key string) bool {
	switch key {
	case "worker_id", "status", "total_items", "succeeded_items", "failed_items", "lease_seconds":
		return true
	default:
		return false
	}
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	default:
		return 0
	}
}
