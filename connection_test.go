package rabbitmq

import (
	"context"
	"errors"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
)

func TestNewConnection_InvalidURL(t *testing.T) {
	conn, err := NewConnection("")
	assert.Error(t, err)
	assert.Nil(t, conn)
	assert.Contains(t, err.Error(), "URL is required")
}

func TestNewConnection_WithOptions(t *testing.T) {
	t.Skip("Requires running RabbitMQ instance")

	conn, err := NewConnection("amqp://guest:guest@localhost:5672/",
		WithCustomQueueDeclare(&QueueDeclareOptions{
			Name:       "test-queue",
			Durable:    true,
			AutoDelete: false,
		}),
		WithCustomPublishOptions(&PublishOptions{
			Key: "test-queue",
		}),
	)
	assert.NoError(t, err)
	assert.NotNil(t, conn)
	defer conn.Close()
}

func TestWithCustomQueueDeclare_Nil(t *testing.T) {
	opt := WithCustomQueueDeclare(nil)
	c := &Connection{}
	err := opt(c)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "queue options cannot be nil")
}

func TestWithCustomPublishOptions_Nil(t *testing.T) {
	opt := WithCustomPublishOptions(nil)
	c := &Connection{}
	err := opt(c)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "publish options cannot be nil")
}

func TestWithReconnectOptions_Nil(t *testing.T) {
	opt := WithReconnectOptions(nil)
	c := &Connection{}
	err := opt(c)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reconnect options cannot be nil")
}

func TestWithConsumerOptions_Nil(t *testing.T) {
	opt := WithConsumerOptions(nil)
	c := &Connection{}
	err := opt(c)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "consumer options cannot be nil")
}

func TestConnection_Close(t *testing.T) {
	c := &Connection{
		url:         "amqp://localhost",
		closeChan:   make(chan struct{}),
		reconnectCh: make(chan struct{}, 1),
		dialFunc: func(url string) (*amqp.Connection, error) {
			return nil, errors.New("mock connection")
		},
	}

	// Close should not error even if connection failed
	err := c.Close()
	assert.NoError(t, err)

	// Second close should also not error
	err = c.Close()
	assert.NoError(t, err)
}

func TestConnection_PublishWithContext_NoChannel(t *testing.T) {
	c := &Connection{
		publishOpts: &PublishOptions{},
		queueOpts:   &QueueDeclareOptions{},
	}

	ctx := context.Background()
	err := c.PublishWithContext(ctx, []byte("test"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "channel is not available")
}

func TestConnection_Consume_NilHandler(t *testing.T) {
	c := &Connection{}
	err := c.Consume(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "message handler is required")
}

// Integration test
func TestConnection_PublishAndConsume_Integration(t *testing.T) {
	t.Skip("Requires running RabbitMQ instance - run with integration tag")

	queueName := "test-integration-queue"

	// Create connection
	conn, err := NewConnection("amqp://guest:guest@localhost:5672/",
		WithCustomQueueDeclare(&QueueDeclareOptions{
			Name:       queueName,
			Durable:    false,
			AutoDelete: true,
		}),
		WithCustomPublishOptions(&PublishOptions{
			Key: queueName,
		}),
		WithConsumerOptions(&ConsumerOptions{
			Queue:         queueName,
			AutoAck:       false,
			PrefetchCount: 5,
		}),
	)
	assert.NoError(t, err)
	defer conn.Close()

	// Setup consumer
	received := make(chan string, 1)
	handler := func(ctx context.Context, msg amqp.Delivery) error {
		received <- string(msg.Body)
		return nil
	}

	err = conn.Consume(handler)
	assert.NoError(t, err)

	// Publish message
	ctx := context.Background()
	testMessage := "test message"
	err = conn.PublishWithContext(ctx, []byte(testMessage))
	assert.NoError(t, err)

	// Wait for message
	select {
	case msg := <-received:
		assert.Equal(t, testMessage, msg)
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}

func TestConnection_Reconnection_Integration(t *testing.T) {
	t.Skip("Requires manual RabbitMQ restart - run manually")

	conn, err := NewConnection("amqp://guest:guest@localhost:5672/",
		WithReconnectOptions(&ReconnectOptions{
			InitialBackoff:    1 * time.Second,
			MaxBackoff:        5 * time.Second,
			BackoffMultiplier: 2.0,
			MaxRetries:        0,
		}),
	)
	assert.NoError(t, err)
	defer conn.Close()

	// Instructions: Stop and start RabbitMQ to test reconnection
	time.Sleep(30 * time.Second)
}
