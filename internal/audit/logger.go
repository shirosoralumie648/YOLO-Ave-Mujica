package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

const SystemActor = "system"

type Event struct {
	Actor        string
	Action       string
	ResourceType string
	ResourceID   string
	Detail       map[string]any
}

type Logger interface {
	Record(ctx context.Context, event Event) error
}

type Recorder struct {
	mu     sync.Mutex
	events []Event
}

func NewRecorder() *Recorder {
	return &Recorder{}
}

func (r *Recorder) Record(_ context.Context, event Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, normalizeEvent(event))
	return nil
}

func (r *Recorder) Events() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Event, len(r.events))
	copy(out, r.events)
	return out
}

type PostgresLogger struct {
	pool *pgxpool.Pool
}

func NewPostgresLogger(pool *pgxpool.Pool) *PostgresLogger {
	return &PostgresLogger{pool: pool}
}

func (l *PostgresLogger) Record(ctx context.Context, event Event) error {
	if l == nil || l.pool == nil {
		return fmt.Errorf("audit logger is not configured")
	}

	event = normalizeEvent(event)
	detailJSON, err := json.Marshal(event.Detail)
	if err != nil {
		return err
	}

	_, err = l.pool.Exec(ctx, `
		insert into audit_logs (actor, action, resource_type, resource_id, detail_json)
		values ($1, $2, $3, $4, $5::jsonb)
	`, event.Actor, event.Action, event.ResourceType, event.ResourceID, detailJSON)
	return err
}

func NormalizeActor(actor string) string {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return SystemActor
	}
	return actor
}

func normalizeEvent(event Event) Event {
	event.Actor = NormalizeActor(event.Actor)
	event.Action = strings.TrimSpace(event.Action)
	event.ResourceType = strings.TrimSpace(event.ResourceType)
	event.ResourceID = strings.TrimSpace(event.ResourceID)
	if event.Detail == nil {
		event.Detail = map[string]any{}
	}
	return event
}
