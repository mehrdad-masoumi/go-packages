package outbox

import (
	"context"
	"log"
	"os"
	"time"
)

// Event is the minimal outbox row the relay needs to publish.
type Event struct {
	ID         string
	Exchange   string
	RoutingKey string
	Payload    []byte
	Attempts   int
}

// Store is implemented by each service's outbox repository adapter.
type Store interface {
	ClaimBatch(ctx context.Context, batchSize int, lockedBy string) ([]Event, error)
	MarkPublished(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string, lastError string, availableAt time.Time) error
	RecoverStuckLocks(ctx context.Context, lockTimeout time.Duration) (int64, error)
}

// Publisher publishes a single message with broker confirms.
type Publisher interface {
	PublishWithConfirm(ctx context.Context, exchange, routingKey string, body []byte) error
}

// Config tunes the outbox relay loop.
type Config struct {
	BatchSize        int
	PollInterval     time.Duration
	LockTimeout      time.Duration
	MaxAttempts      int
	RecoveryInterval time.Duration
	BaseBackoff      time.Duration
	MaxBackoff       time.Duration
}

// Hooks optional callbacks (e.g. Prometheus counters).
type Hooks struct {
	OnPublished func()
	OnFailed    func()
}

// Option configures PublisherService.
type Option func(*PublisherService)

// WithDefaultExchange sets the exchange used when Event.Exchange is empty.
func WithDefaultExchange(exchange string) Option {
	return func(p *PublisherService) { p.defaultExchange = exchange }
}

// WithLockedBy sets the lock owner id (defaults to hostname).
func WithLockedBy(name string) Option {
	return func(p *PublisherService) { p.lockedBy = name }
}

// WithHooks registers success/failure callbacks.
func WithHooks(h Hooks) Option {
	return func(p *PublisherService) { p.hooks = h }
}

// PublisherService relays claimed outbox rows to a message broker.
type PublisherService struct {
	store           Store
	queue           Publisher
	cfg             Config
	lockedBy        string
	defaultExchange string
	hooks           Hooks
}

// New builds a PublisherService.
func New(store Store, queue Publisher, cfg Config, opts ...Option) *PublisherService {
	if cfg.RecoveryInterval <= 0 {
		cfg.RecoveryInterval = 30 * time.Second
	}
	if cfg.BaseBackoff <= 0 {
		cfg.BaseBackoff = time.Second
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 5 * time.Minute
	}
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "outbox"
	}
	p := &PublisherService{
		store:    store,
		queue:    queue,
		cfg:      cfg,
		lockedBy: hostname,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Run blocks until ctx is cancelled.
func (p *PublisherService) Run(ctx context.Context) {
	poll := time.NewTicker(p.cfg.PollInterval)
	defer poll.Stop()
	recovery := time.NewTicker(p.cfg.RecoveryInterval)
	defer recovery.Stop()

	p.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-poll.C:
			p.tick(ctx)
		case <-recovery.C:
			if n, err := p.store.RecoverStuckLocks(ctx, p.cfg.LockTimeout); err != nil {
				log.Printf("outbox recover: %v", err)
			} else if n > 0 {
				log.Printf("outbox recover: reset %d stuck rows", n)
			}
		}
	}
}

func (p *PublisherService) tick(ctx context.Context) {
	events, err := p.store.ClaimBatch(ctx, p.cfg.BatchSize, p.lockedBy)
	if err != nil {
		log.Printf("outbox claim: %v", err)
		return
	}
	for _, e := range events {
		if err := p.publishOne(ctx, e); err != nil {
			if p.hooks.OnFailed != nil {
				p.hooks.OnFailed()
			}
			p.handleFailure(ctx, e, err)
			continue
		}
		if p.hooks.OnPublished != nil {
			p.hooks.OnPublished()
		}
		if err := p.store.MarkPublished(ctx, e.ID); err != nil {
			log.Printf("outbox mark published %s: %v", e.ID, err)
		}
	}
}

func (p *PublisherService) publishOne(ctx context.Context, e Event) error {
	pubCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	exchange := e.Exchange
	if exchange == "" {
		exchange = p.defaultExchange
	}
	return p.queue.PublishWithConfirm(pubCtx, exchange, e.RoutingKey, e.Payload)
}

func (p *PublisherService) handleFailure(ctx context.Context, e Event, pubErr error) {
	attempt := e.Attempts + 1
	msg := pubErr.Error()
	if len(msg) > 500 {
		msg = msg[:500]
	}
	delay := Backoff(attempt, p.cfg.BaseBackoff, p.cfg.MaxBackoff)
	if p.cfg.MaxAttempts > 0 && attempt >= p.cfg.MaxAttempts {
		delay = 24 * time.Hour
		msg = "max attempts exceeded: " + msg
	}
	if err := p.store.MarkFailed(ctx, e.ID, msg, time.Now().Add(delay)); err != nil {
		log.Printf("outbox mark failed %s: %v", e.ID, err)
	}
}

// Backoff returns exponential backoff capped at max.
func Backoff(attempt int, base, max time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := base
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= max {
			return max
		}
	}
	return d
}
