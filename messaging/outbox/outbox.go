package outbox

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"
)

type Event struct {
	ID         string
	Exchange   string
	RoutingKey string
	Payload    []byte
	Attempts   int
}

type Store interface {
	ClaimBatch(ctx context.Context, batchSize int, lockedBy string) ([]Event, error)
	MarkPublished(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id, lastError string, availableAt time.Time) error
	RecoverStuckLocks(ctx context.Context, lockTimeout time.Duration) (int64, error)
}

type Publisher interface {
	PublishWithConfirm(ctx context.Context, exchange, routingKey string, body []byte) error
}

type Hooks struct {
	OnPublished     func(Event)
	OnPublishFailed func(Event, error)
	OnError         func(stage string, error error)
}

type Config struct {
	BatchSize        int
	PollInterval     time.Duration
	LockTimeout      time.Duration
	MaxAttempts      int
	RecoveryInterval time.Duration
	BaseBackoff      time.Duration
	MaxBackoff       time.Duration
	PublishTimeout   time.Duration
	LockedBy         string
	DefaultExchange  string
	Logger           *slog.Logger
	Hooks            Hooks
}

type Relay struct {
	store     Store
	publisher Publisher
	cfg       Config
}

func New(store Store, publisher Publisher, cfg Config) (*Relay, error) {
	if store == nil {
		return nil, errors.New("outbox: store is required")
	}
	if publisher == nil {
		return nil, errors.New("outbox: publisher is required")
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = time.Second
	}
	if cfg.LockTimeout <= 0 {
		cfg.LockTimeout = time.Minute
	}
	if cfg.RecoveryInterval <= 0 {
		cfg.RecoveryInterval = 30 * time.Second
	}
	if cfg.BaseBackoff <= 0 {
		cfg.BaseBackoff = time.Second
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 5 * time.Minute
	}
	if cfg.PublishTimeout <= 0 {
		cfg.PublishTimeout = 10 * time.Second
	}
	if cfg.LockedBy == "" {
		cfg.LockedBy, _ = os.Hostname()
		if cfg.LockedBy == "" {
			cfg.LockedBy = "outbox"
		}
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Relay{store: store, publisher: publisher, cfg: cfg}, nil
}

func (r *Relay) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("outbox: context is required")
	}
	poll := time.NewTicker(r.cfg.PollInterval)
	recovery := time.NewTicker(r.cfg.RecoveryInterval)
	defer poll.Stop()
	defer recovery.Stop()
	r.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-poll.C:
			r.tick(ctx)
		case <-recovery.C:
			r.recover(ctx)
		}
	}
}

func (r *Relay) tick(ctx context.Context) {
	events, err := r.store.ClaimBatch(ctx, r.cfg.BatchSize, r.cfg.LockedBy)
	if err != nil {
		r.report(ctx, "claim", err)
		return
	}
	for _, event := range events {
		if ctx.Err() != nil {
			return
		}
		if err := r.publishOne(ctx, event); err != nil {
			if r.cfg.Hooks.OnPublishFailed != nil {
				r.cfg.Hooks.OnPublishFailed(event, err)
			}
			r.fail(ctx, event, err)
			continue
		}
		if err := r.store.MarkPublished(ctx, event.ID); err != nil {
			// At-least-once delivery means this can be retried and duplicated; consumers must be idempotent.
			r.report(ctx, "mark_published", err, "event_id", event.ID)
			continue
		}
		if r.cfg.Hooks.OnPublished != nil {
			r.cfg.Hooks.OnPublished(event)
		}
	}
}

func (r *Relay) publishOne(ctx context.Context, event Event) error {
	pubCtx, cancel := context.WithTimeout(ctx, r.cfg.PublishTimeout)
	defer cancel()
	exchange := event.Exchange
	if exchange == "" {
		exchange = r.cfg.DefaultExchange
	}
	return r.publisher.PublishWithConfirm(pubCtx, exchange, event.RoutingKey, event.Payload)
}

func (r *Relay) fail(ctx context.Context, event Event, pubErr error) {
	attempt := event.Attempts + 1
	msg := pubErr.Error()
	if len(msg) > 500 {
		msg = msg[:500]
	}
	delay := Backoff(attempt, r.cfg.BaseBackoff, r.cfg.MaxBackoff)
	if r.cfg.MaxAttempts > 0 && attempt >= r.cfg.MaxAttempts {
		msg = "max attempts exceeded: " + msg
		delay = r.cfg.MaxBackoff
	}
	if err := r.store.MarkFailed(ctx, event.ID, msg, time.Now().Add(delay)); err != nil {
		r.report(ctx, "mark_failed", err, "event_id", event.ID)
	}
}

func (r *Relay) recover(ctx context.Context) {
	n, err := r.store.RecoverStuckLocks(ctx, r.cfg.LockTimeout)
	if err != nil {
		r.report(ctx, "recover", err)
		return
	}
	if n > 0 {
		r.cfg.Logger.InfoContext(ctx, "outbox recovered stuck locks", "count", n)
	}
}

func (r *Relay) report(ctx context.Context, stage string, err error, args ...any) {
	if r.cfg.Hooks.OnError != nil {
		r.cfg.Hooks.OnError(stage, err)
	}
	fields := append([]any{"stage", stage, "error", err.Error()}, args...)
	r.cfg.Logger.ErrorContext(ctx, "outbox relay error", fields...)
}

func Backoff(attempt int, base, max time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if base <= 0 {
		base = time.Second
	}
	if max <= 0 {
		max = 5 * time.Minute
	}
	delay := base
	for i := 1; i < attempt; i++ {
		if delay >= max || delay > max/2 {
			return max
		}
		delay *= 2
	}
	if delay > max {
		return max
	}
	return delay
}
