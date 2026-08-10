package rabbitmq

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/mehrdad-masoumi/go-packages/observability/tracing"
)

const defaultConfirmTimeout = 5 * time.Second
const defaultContentType = "application/json"

// TopologySetup declares exchanges/queues/bindings for a service.
type TopologySetup func(ch *amqp.Channel) error

// Client manages a reconnecting AMQP connection and confirmed publishes.
type Client struct {
	url            string
	setup          TopologySetup
	contentType    string
	confirmTimeout time.Duration
	conn           *amqp.Connection
	mu             sync.Mutex
}

// Option configures Client.
type Option func(*Client)

// WithContentType sets the AMQP content-type header (default application/json).
func WithContentType(ct string) Option {
	return func(c *Client) {
		if ct != "" {
			c.contentType = ct
		}
	}
}

// WithConfirmTimeout sets publish confirm wait timeout.
func WithConfirmTimeout(d time.Duration) Option {
	return func(c *Client) {
		if d > 0 {
			c.confirmTimeout = d
		}
	}
}

// New dials RabbitMQ, runs topology setup, and starts reconnect loop.
func New(url string, setup TopologySetup, opts ...Option) (*Client, error) {
	c := &Client{
		url:            url,
		setup:          setup,
		contentType:    defaultContentType,
		confirmTimeout: defaultConfirmTimeout,
	}
	for _, opt := range opts {
		opt(c)
	}
	if err := c.connect(); err != nil {
		return nil, err
	}
	if c.setup != nil {
		if err := c.SetupTopology(); err != nil {
			_ = c.Close()
			return nil, err
		}
	}
	go c.handleReconnect()
	return c, nil
}

func (c *Client) connect() error {
	conn, err := amqp.Dial(c.url)
	if err != nil {
		return fmt.Errorf("rabbitmq dial: %w", err)
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	return nil
}

func (c *Client) handleReconnect() {
	for {
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()
		if conn == nil {
			time.Sleep(5 * time.Second)
			continue
		}
		errCh := conn.NotifyClose(make(chan *amqp.Error, 1))
		err := <-errCh
		log.Printf("rabbitmq connection closed: %v; reconnecting", err)
		for {
			if err := c.connect(); err != nil {
				log.Printf("rabbitmq reconnect failed: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}
			if c.setup != nil {
				if err := c.SetupTopology(); err != nil {
					log.Printf("rabbitmq topology setup failed: %v", err)
					time.Sleep(5 * time.Second)
					continue
				}
			}
			log.Printf("rabbitmq reconnected")
			break
		}
	}
}

// Channel opens a new channel on the current connection.
func (c *Client) Channel() (*amqp.Channel, error) {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil || conn.IsClosed() {
		return nil, fmt.Errorf("rabbitmq not connected")
	}
	return conn.Channel()
}

// SetupTopology runs the configured topology callback.
func (c *Client) SetupTopology() error {
	if c.setup == nil {
		return nil
	}
	ch, err := c.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	return c.setup(ch)
}

// PublishWithConfirm publishes with publisher confirms and injects W3C trace headers.
func (c *Client) PublishWithConfirm(ctx context.Context, exchange, routingKey string, body []byte) error {
	return c.PublishWithConfirmHeaders(ctx, exchange, routingKey, body, nil)
}

// PublishWithConfirmHeaders publishes with confirms, merges headers, and injects W3C trace context.
func (c *Client) PublishWithConfirmHeaders(ctx context.Context, exchange, routingKey string, body []byte, headers amqp.Table) error {
	ch, err := c.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	if err := ch.Confirm(false); err != nil {
		return fmt.Errorf("confirm mode: %w", err)
	}
	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))

	ctx = tracing.ExtractFromJSONCarrier(ctx, body)
	ctx, span, headers := tracing.StartProducerSpan(ctx, exchange, routingKey, headers)
	var pubErr error
	defer func() { tracing.EndSpan(span, pubErr) }()

	pubCtx, cancel := context.WithTimeout(ctx, c.confirmTimeout)
	defer cancel()

	if err := ch.PublishWithContext(pubCtx, exchange, routingKey, false, false, amqp.Publishing{
		ContentType:  c.contentType,
		DeliveryMode: amqp.Persistent,
		Body:         body,
		Timestamp:    time.Now().UTC(),
		Headers:      headers,
	}); err != nil {
		pubErr = err
		return err
	}

	select {
	case <-pubCtx.Done():
		pubErr = pubCtx.Err()
		return pubErr
	case conf, ok := <-confirms:
		if !ok {
			pubErr = fmt.Errorf("confirm channel closed")
			return pubErr
		}
		if !conf.Ack {
			pubErr = fmt.Errorf("publish nacked")
			return pubErr
		}
		return nil
	}
}

// Ping opens and closes a channel to verify connectivity.
func (c *Client) Ping(ctx context.Context) error {
	_ = ctx
	ch, err := c.Channel()
	if err != nil {
		return err
	}
	return ch.Close()
}

// Close closes the underlying connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
