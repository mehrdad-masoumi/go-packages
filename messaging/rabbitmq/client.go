package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/mehrdad-masoumi/go-packages/observability/logger"
	"github.com/mehrdad-masoumi/go-packages/observability/tracing"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	defaultConfirmTimeout = 5 * time.Second
	defaultReconnectDelay = 5 * time.Second
)

type TopologySetup func(ch *amqp.Channel) error

type Config struct {
	URL            string
	ContentType    string
	ConfirmTimeout time.Duration
	ReconnectDelay time.Duration
	DialTimeout    time.Duration
}

type Client struct {
	cfg       Config
	setup     TopologySetup
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.RWMutex
	conn      *amqp.Connection
	wg        sync.WaitGroup
	closeOnce sync.Once
}

func New(ctx context.Context, cfg Config, setup TopologySetup) (*Client, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg.URL == "" {
		return nil, errors.New("rabbitmq: URL is required")
	}
	if cfg.ContentType == "" {
		cfg.ContentType = "application/json"
	}
	if cfg.ConfirmTimeout <= 0 {
		cfg.ConfirmTimeout = defaultConfirmTimeout
	}
	if cfg.ReconnectDelay <= 0 {
		cfg.ReconnectDelay = defaultReconnectDelay
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 5 * time.Second
	}
	runCtx, cancel := context.WithCancel(ctx)
	client := &Client{cfg: cfg, setup: setup, ctx: runCtx, cancel: cancel}
	if err := client.connectAndSetup(); err != nil {
		cancel()
		return nil, err
	}
	client.wg.Add(1)
	go client.reconnectLoop()
	return client, nil
}

func (c *Client) connectAndSetup() error {
	conn, err := amqp.DialConfig(c.cfg.URL, amqp.Config{
		Dial: func(network, addr string) (net.Conn, error) {
			return net.DialTimeout(network, addr, c.cfg.DialTimeout)
		},
	})
	if err != nil {
		return fmt.Errorf("rabbitmq dial: %w", err)
	}
	if c.setup != nil {
		ch, chErr := conn.Channel()
		if chErr != nil {
			_ = conn.Close()
			return fmt.Errorf("rabbitmq topology channel: %w", chErr)
		}
		setupErr := c.setup(ch)
		_ = ch.Close()
		if setupErr != nil {
			_ = conn.Close()
			return fmt.Errorf("rabbitmq topology: %w", setupErr)
		}
	}
	c.mu.Lock()
	old := c.conn
	c.conn = conn
	c.mu.Unlock()
	if old != nil && !old.IsClosed() {
		_ = old.Close()
	}
	return nil
}

func (c *Client) reconnectLoop() {
	defer c.wg.Done()
	for {
		conn := c.currentConnection()
		if conn == nil {
			if !c.waitReconnect() {
				return
			}
			c.tryReconnect()
			continue
		}
		closeCh := conn.NotifyClose(make(chan *amqp.Error, 1))
		select {
		case <-c.ctx.Done():
			return
		case err := <-closeCh:
			if c.ctx.Err() != nil {
				return
			}
			if err != nil {
				logger.Warn(c.ctx, "rabbitmq connection closed", "error", err.Error())
			}
		}
		c.tryReconnect()
	}
}

func (c *Client) tryReconnect() {
	for c.ctx.Err() == nil {
		if err := c.connectAndSetup(); err != nil {
			logger.Error(c.ctx, "rabbitmq reconnect failed", "error", err.Error())
			if !c.waitReconnect() {
				return
			}
			continue
		}
		logger.Info(c.ctx, "rabbitmq reconnected")
		return
	}
}

func (c *Client) waitReconnect() bool {
	timer := time.NewTimer(c.cfg.ReconnectDelay)
	defer timer.Stop()
	select {
	case <-c.ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (c *Client) currentConnection() *amqp.Connection {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn
}

func (c *Client) Channel() (*amqp.Channel, error) {
	conn := c.currentConnection()
	if conn == nil || conn.IsClosed() {
		return nil, errors.New("rabbitmq not connected")
	}
	return conn.Channel()
}

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

func (c *Client) PublishWithConfirm(ctx context.Context, exchange, routingKey string, body []byte) error {
	return c.PublishWithConfirmHeaders(ctx, exchange, routingKey, body, nil)
}

func (c *Client) PublishWithConfirmHeaders(ctx context.Context, exchange, routingKey string, body []byte, headers amqp.Table) (pubErr error) {
	ch, err := c.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	if err := ch.Confirm(false); err != nil {
		return fmt.Errorf("rabbitmq confirm mode: %w", err)
	}
	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))

	ctx = tracing.ExtractFromJSONCarrier(ctx, body)
	ctx, span, headers := startProducerSpan(ctx, exchange, routingKey, headers)
	defer func() { endSpan(span, pubErr) }()
	pubCtx, cancel := context.WithTimeout(ctx, c.cfg.ConfirmTimeout)
	defer cancel()

	if err := ch.PublishWithContext(pubCtx, exchange, routingKey, false, false, amqp.Publishing{
		ContentType: c.cfg.ContentType, DeliveryMode: amqp.Persistent, Body: body,
		Timestamp: time.Now().UTC(), Headers: headers,
	}); err != nil {
		pubErr = err
		return fmt.Errorf("rabbitmq publish: %w", err)
	}

	select {
	case <-pubCtx.Done():
		pubErr = pubCtx.Err()
		return pubErr
	case confirmation, ok := <-confirms:
		if !ok {
			pubErr = errors.New("rabbitmq confirm channel closed")
			return pubErr
		}
		if !confirmation.Ack {
			pubErr = errors.New("rabbitmq publish nacked")
			return pubErr
		}
		return nil
	}
}

func (c *Client) Ping(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	ch, err := c.Channel()
	if err != nil {
		return err
	}
	return ch.Close()
}

func (c *Client) Close() error {
	var closeErr error
	c.closeOnce.Do(func() {
		c.cancel()
		c.mu.Lock()
		conn := c.conn
		c.conn = nil
		c.mu.Unlock()
		if conn != nil {
			closeErr = conn.Close()
		}
		c.wg.Wait()
	})
	return closeErr
}
