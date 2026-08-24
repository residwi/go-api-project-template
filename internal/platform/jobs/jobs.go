package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrDiscard = errors.New("discard job")

type Job interface {
	Kind() string
	Run(ctx context.Context) error
}

type Keys struct {
	Dedup string
	Group string
}

type Record struct {
	ID          uuid.UUID
	Queue       string
	Kind        string
	Payload     []byte
	DedupKey    string
	GroupKey    string
	Status      string
	Attempts    int
	MaxAttempts int
	LastError   string
	LockedUntil *time.Time
	RunAt       time.Time
}

type Enqueuer interface {
	Insert(ctx context.Context, r Record) error
}

type Queue interface {
	Enqueuer
	CancelByGroupKey(ctx context.Context, groupKey string) (int, error)
}

type Store interface {
	Insert(ctx context.Context, r Record) error
	Claim(ctx context.Context, queue string, batch int, lease time.Duration) ([]Record, error)
	Complete(ctx context.Context, id uuid.UUID) error
	Retry(ctx context.Context, id uuid.UUID, attempts int, lastErr string, runAt time.Time) error
	Bury(ctx context.Context, id uuid.UUID, attempts int, lastErr string) error
	Cancel(ctx context.Context, id uuid.UUID, lastErr string) error
	CancelByDedupKey(ctx context.Context, dedupKey string) (int, error)
	CancelByGroupKey(ctx context.Context, groupKey string) (int, error)
	Prune(ctx context.Context, queue string, age time.Duration, limit int) (int, error)
}

type Option func(*Record)

func MaxAttempts(n int) Option {
	return func(r *Record) { r.MaxAttempts = n }
}

func RunAt(t time.Time) Option {
	return func(r *Record) { r.RunAt = t }
}

func Enqueue[T Job](ctx context.Context, e Enqueuer, job T, keys Keys, opts ...Option) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshalling job payload: %w", err)
	}

	kind := job.Kind()
	rec := Record{
		Queue:       queueOf(kind),
		Kind:        kind,
		Payload:     payload,
		DedupKey:    keys.Dedup,
		GroupKey:    keys.Group,
		Status:      "pending",
		MaxAttempts: defaultMaxAttempts,
		RunAt:       time.Now(),
	}
	for _, opt := range opts {
		opt(&rec)
	}

	return e.Insert(ctx, rec)
}

type Registry struct {
	handlers map[string]func(context.Context, []byte) error
}

func NewRegistry() *Registry {
	return &Registry{handlers: make(map[string]func(context.Context, []byte) error)}
}

func Register[T Job](r *Registry, proto T) {
	r.handlers[proto.Kind()] = func(ctx context.Context, payload []byte) error {
		job := proto
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &job); err != nil {
				return fmt.Errorf("unmarshalling job payload: %w", err)
			}
		}
		return job.Run(ctx)
	}
}

func (r *Registry) Process(ctx context.Context, rec Record) error {
	handler, ok := r.handlers[rec.Kind]
	if !ok {
		return fmt.Errorf("no handler registered for kind %s: %w", rec.Kind, ErrDiscard)
	}
	return handler(ctx, rec.Payload)
}

const defaultMaxAttempts = 3

func queueOf(kind string) string {
	if queue, _, found := strings.Cut(kind, "."); found {
		return queue
	}
	return kind
}
