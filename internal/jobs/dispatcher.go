package jobs

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/redis/go-redis/v9"
)

type Publisher interface {
	Publish(ctx context.Context, lane string, payload map[string]any) error
}

type RedisPublisher struct {
	client *redis.Client
}

func NewRedisPublisher(client *redis.Client) *RedisPublisher {
	return &RedisPublisher{client: client}
}

func (p *RedisPublisher) Publish(ctx context.Context, lane string, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return p.client.LPush(ctx, lane, body).Err()
}

func laneFor(resource string) string {
	switch resource {
	case "gpu":
		return "jobs:gpu"
	case "mixed":
		return "jobs:mixed"
	default:
		return "jobs:cpu"
	}
}

func buildDispatchPayload(job *Job) map[string]any {
	return map[string]any{
		"job_id":                 job.ID,
		"project_id":             job.ProjectID,
		"dataset_id":             job.DatasetID,
		"snapshot_id":            job.SnapshotID,
		"job_type":               job.JobType,
		"resource_lane":          laneFor(job.RequiredResourceType),
		"required_resource_type": job.RequiredResourceType,
		"required_capabilities":  job.RequiredCapabilities,
		"payload":                job.Payload,
	}
}

type InMemoryPublisher struct {
	mu       sync.Mutex
	lastLane string
	items    []PublishedItem
}

type PublishedItem struct {
	Lane    string
	Payload map[string]any
}

func NewInMemoryPublisher() *InMemoryPublisher {
	return &InMemoryPublisher{items: []PublishedItem{}}
}

func (p *InMemoryPublisher) Publish(_ context.Context, lane string, payload map[string]any) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	cp := make(map[string]any, len(payload))
	for k, v := range payload {
		cp[k] = v
	}
	p.lastLane = lane
	p.items = append(p.items, PublishedItem{Lane: lane, Payload: cp})
	return nil
}

func (p *InMemoryPublisher) LastLane() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastLane
}
