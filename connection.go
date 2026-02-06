package rabbitmq

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ConnectionOption is a functional option for configuring the connection
type ConnectionOption func(*Connection) error

// Connection manages a RabbitMQ connection with automatic reconnection
type Connection struct {
	url  string
	conn *amqp.Connection
	mu   sync.RWMutex

	// Channels
	pubCh  *amqp.Channel
	consCh *amqp.Channel

	// Configuration
	queueOpts     *QueueDeclareOptions
	publishOpts   *PublishOptions
	consumerOpts  *ConsumerOptions
	reconnectOpts *ReconnectOptions

	// State
	closed        bool
	closeChan     chan struct{}
	reconnectCh   chan struct{}
	notifyCloseCh chan *amqp.Error

	// Consumer
	consumerHandler MessageHandler
	consumerWg      sync.WaitGroup

	// For testing
	dialFunc func(string) (*amqp.Connection, error)
}

// PublishOptions holds options for publishing
type PublishOptions struct {
	Key       string
	Mandatory bool
	Immediate bool
}

// ConsumerOptions holds options for consuming
type ConsumerOptions struct {
	Queue         string
	ConsumerTag   string
	AutoAck       bool
	Exclusive     bool
	PrefetchCount int
}

// ReconnectOptions holds reconnection settings
type ReconnectOptions struct {
	MaxRetries        int
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	BackoffMultiplier float64
}

// MessageHandler processes consumed messages
type MessageHandler func(context.Context, amqp.Delivery) error

// NewConnection creates a new RabbitMQ connection with options
func NewConnection(url string, opts ...ConnectionOption) (*Connection, error) {
	if url == "" {
		return nil, fmt.Errorf("URL is required")
	}

	c := &Connection{
		url:         url,
		closeChan:   make(chan struct{}),
		reconnectCh: make(chan struct{}, 1),
		dialFunc:    amqp.Dial,

		// Defaults
		queueOpts: &QueueDeclareOptions{
			Durable:    true,
			AutoDelete: false,
			Exclusive:  false,
		},
		publishOpts: &PublishOptions{
			Key:       "",
			Mandatory: false,
			Immediate: false,
		},
		consumerOpts: &ConsumerOptions{
			AutoAck:       false,
			Exclusive:     false,
			PrefetchCount: 10,
		},
		reconnectOpts: &ReconnectOptions{
			MaxRetries:        0, // Infinite
			InitialBackoff:    1 * time.Second,
			MaxBackoff:        30 * time.Second,
			BackoffMultiplier: 2.0,
		},
	}

	// Apply options
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	// Connect
	if err := c.connect(); err != nil {
		return nil, err
	}

	// Start reconnection monitor
	go c.reconnectLoop()

	return c, nil
}

// WithCustomQueueDeclare sets custom queue declaration options
func WithCustomQueueDeclare(opts *QueueDeclareOptions) ConnectionOption {
	return func(c *Connection) error {
		if opts == nil {
			return fmt.Errorf("queue options cannot be nil")
		}
		c.queueOpts = opts
		return nil
	}
}

// WithCustomPublishOptions sets custom publish options
func WithCustomPublishOptions(opts *PublishOptions) ConnectionOption {
	return func(c *Connection) error {
		if opts == nil {
			return fmt.Errorf("publish options cannot be nil")
		}
		c.publishOpts = opts
		return nil
	}
}

// WithReconnectOptions sets custom reconnection options
func WithReconnectOptions(opts *ReconnectOptions) ConnectionOption {
	return func(c *Connection) error {
		if opts == nil {
			return fmt.Errorf("reconnect options cannot be nil")
		}
		c.reconnectOpts = opts
		return nil
	}
}

// WithConsumerOptions sets custom consumer options
func WithConsumerOptions(opts *ConsumerOptions) ConnectionOption {
	return func(c *Connection) error {
		if opts == nil {
			return fmt.Errorf("consumer options cannot be nil")
		}
		c.consumerOpts = opts
		return nil
	}
}

// connect establishes connection and creates channels
func (c *Connection) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Dial connection
	conn, err := c.dialFunc(c.url)
	if err != nil {
		return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	c.conn = conn
	c.notifyCloseCh = make(chan *amqp.Error, 1)
	c.conn.NotifyClose(c.notifyCloseCh)

	// Create publisher channel
	pubCh, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create publisher channel: %w", err)
	}
	c.pubCh = pubCh

	// Declare queue if name is set
	if c.queueOpts.Name != "" {
		_, err := DeclareQuorumQueue(pubCh, c.queueOpts)
		if err != nil {
			// Check if it's a 406 error - channel is closed, need to recreate
			if amqpErr, ok := err.(*amqp.Error); ok && amqpErr.Code == 406 {
				log.Printf("Channel closed due to 406 error, recreating channel for fallback attempt")

				// Recreate the channel
				pubCh, err = conn.Channel()
				if err != nil {
					conn.Close()
					return fmt.Errorf("failed to recreate publisher channel: %w", err)
				}
				c.pubCh = pubCh

				// Try again with classic queue (no x-queue-type)
				classicOpts := &QueueDeclareOptions{
					Name:       c.queueOpts.Name,
					Durable:    c.queueOpts.Durable,
					AutoDelete: c.queueOpts.AutoDelete,
					Exclusive:  c.queueOpts.Exclusive,
					NoWait:     c.queueOpts.NoWait,
					Args:       amqp.Table{},
				}

				// Copy args except x-queue-type
				if c.queueOpts.Args != nil {
					for k, v := range c.queueOpts.Args {
						if k != amqp.QueueTypeArg {
							classicOpts.Args[k] = v
						}
					}
				}

				_, err = pubCh.QueueDeclare(
					classicOpts.Name,
					classicOpts.Durable,
					classicOpts.AutoDelete,
					classicOpts.Exclusive,
					classicOpts.NoWait,
					classicOpts.Args,
				)

				if err != nil {
					conn.Close()
					return fmt.Errorf("failed to declare classic queue after fallback: %w", err)
				}

				log.Printf("Successfully declared classic queue after fallback: %s", c.queueOpts.Name)
			} else {
				conn.Close()
				return fmt.Errorf("failed to declare queue: %w", err)
			}
		}
	}

	log.Println("Successfully connected to RabbitMQ")

	// Restart consumer if handler is set
	if c.consumerHandler != nil {
		if err := c.setupConsumer(); err != nil {
			log.Printf("Failed to setup consumer: %v", err)
		}
	}

	return nil
}

// setupConsumer creates consumer channel and starts consuming
func (c *Connection) setupConsumer() error {
	if c.consumerOpts.Queue == "" {
		c.consumerOpts.Queue = c.queueOpts.Name
	}

	// Create consumer channel
	consCh, err := c.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to create consumer channel: %w", err)
	}

	// Set QoS
	if err := consCh.Qos(c.consumerOpts.PrefetchCount, 0, false); err != nil {
		consCh.Close()
		return fmt.Errorf("failed to set QoS: %w", err)
	}

	c.consCh = consCh

	// Start consuming
	deliveryCh, err := consCh.Consume(
		c.consumerOpts.Queue,
		c.consumerOpts.ConsumerTag,
		c.consumerOpts.AutoAck,
		c.consumerOpts.Exclusive,
		false, // noLocal
		false, // noWait
		nil,   // args
	)
	if err != nil {
		consCh.Close()
		return fmt.Errorf("failed to start consuming: %w", err)
	}

	// Process messages
	c.consumerWg.Add(1)
	go c.processMessages(deliveryCh)

	log.Printf("Started consuming from queue: %s", c.consumerOpts.Queue)
	return nil
}

// processMessages handles incoming messages
func (c *Connection) processMessages(deliveryCh <-chan amqp.Delivery) {
	defer c.consumerWg.Done()

	for {
		select {
		case <-c.closeChan:
			return
		case msg, ok := <-deliveryCh:
			if !ok {
				return
			}
			c.handleMessage(msg)
		}
	}
}

// handleMessage processes a single message
func (c *Connection) handleMessage(msg amqp.Delivery) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic while handling message: %v", r)
			if !c.consumerOpts.AutoAck {
				msg.Nack(false, true)
			}
		}
	}()

	ctx := context.Background()
	if err := c.consumerHandler(ctx, msg); err != nil {
		log.Printf("Error handling message: %v", err)
		if !c.consumerOpts.AutoAck {
			msg.Nack(false, true)
		}
		return
	}

	if !c.consumerOpts.AutoAck {
		if err := msg.Ack(false); err != nil {
			log.Printf("Failed to ack message: %v", err)
		}
	}
}

// reconnectLoop monitors connection and reconnects
func (c *Connection) reconnectLoop() {
	for {
		select {
		case <-c.closeChan:
			return
		case err := <-c.notifyCloseCh:
			if c.isClosed() {
				return
			}
			log.Printf("Connection closed: %v. Attempting to reconnect...", err)
			c.attemptReconnect()
		case <-c.reconnectCh:
			if c.isClosed() {
				return
			}
			c.attemptReconnect()
		}
	}
}

// attemptReconnect reconnects with exponential backoff
func (c *Connection) attemptReconnect() {
	backoff := c.reconnectOpts.InitialBackoff
	attempt := 0

	for {
		if c.isClosed() {
			return
		}

		attempt++
		if c.reconnectOpts.MaxRetries > 0 && attempt > c.reconnectOpts.MaxRetries {
			log.Printf("Max reconnection attempts (%d) reached", c.reconnectOpts.MaxRetries)
			return
		}

		jitter := time.Duration(rand.Float64() * float64(backoff) * 0.1)
		sleepDuration := backoff + jitter

		log.Printf("Reconnection attempt %d in %v...", attempt, sleepDuration)
		time.Sleep(sleepDuration)

		if err := c.connect(); err != nil {
			log.Printf("Reconnection attempt %d failed: %v", attempt, err)
			backoff = time.Duration(float64(backoff) * c.reconnectOpts.BackoffMultiplier)
			if backoff > c.reconnectOpts.MaxBackoff {
				backoff = c.reconnectOpts.MaxBackoff
			}
			continue
		}

		log.Printf("Reconnection attempt %d succeeded", attempt)
		return
	}
}

// PublishWithContext publishes a message
func (c *Connection) PublishWithContext(ctx context.Context, data []byte) error {
	c.mu.RLock()
	ch := c.pubCh
	opts := c.publishOpts
	queueName := c.queueOpts.Name
	c.mu.RUnlock()

	if ch == nil {
		return fmt.Errorf("publisher channel is not available")
	}

	// Determine routing key (use Key from options, or queue name if empty)
	routingKey := opts.Key
	if routingKey == "" {
		routingKey = queueName
	}

	msg := amqp.Publishing{
		ContentType:  "application/json",
		Body:         data,
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
	}

	err := ch.PublishWithContext(
		ctx,
		"", // Default exchange
		routingKey,
		opts.Mandatory,
		opts.Immediate,
		msg,
	)

	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	return nil
}

// Consume starts consuming messages with the provided handler
func (c *Connection) Consume(handler MessageHandler) error {
	if handler == nil {
		return fmt.Errorf("message handler is required")
	}

	c.mu.Lock()
	c.consumerHandler = handler
	c.mu.Unlock()

	return c.setupConsumer()
}

// Close closes the connection gracefully
func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	close(c.closeChan)

	// Wait for consumer to finish
	c.consumerWg.Wait()

	// Close channels
	if c.pubCh != nil {
		c.pubCh.Close()
	}
	if c.consCh != nil {
		c.consCh.Close()
	}

	// Close connection
	if c.conn != nil && !c.conn.IsClosed() {
		if err := c.conn.Close(); err != nil {
			return fmt.Errorf("failed to close connection: %w", err)
		}
	}

	log.Println("Connection closed successfully")
	return nil
}

// isClosed checks if connection is closed
func (c *Connection) isClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}
