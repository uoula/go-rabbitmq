# RabbitMQ Go Library

A production-ready Go library for RabbitMQ with automatic reconnection and a clean functional options API.

## Features

- ✅ **Simple API**: Only `NewConnection`, `PublishWithContext`, and `Consume` methods
- ✅ **Functional Options**: Configure with `WithCustomQueueDeclare` and `WithCustomPublishOptions`
- ✅ **Automatic Reconnection**: Monitors connection health and reconnects automatically with exponential backoff
- ✅ **Queue Declaration**: Smart queue declaration with quorum/classic fallback on 406 errors
- ✅ **Graceful Shutdown**: Properly handles shutdown without losing in-flight messages
- ✅ **Thread-Safe**: All operations are safe for concurrent use
- ✅ **Comprehensive Testing**: Unit tests and integration tests included

## Installation

```bash
go get github.com/uoula/go-rabbitmq
```

## Quick Start

### Publisher Example

```go
package main

import (
    "context"
    "log"
    
    "github.com/uoula/go-rabbitmq"
)

func main() {
    // Create connection with options
    conn, err := rabbitmq.NewConnection("amqp://guest:guest@localhost:5672/",
        rabbitmq.WithCustomQueueDeclare(&rabbitmq.QueueDeclareOptions{
            Name:       "my-queue",
            Durable:    true,
            AutoDelete: false,
        }),
        rabbitmq.WithCustomPublishOptions(&rabbitmq.PublishOptions{
            Key: "my-queue", // Routing key (optional, defaults to queue name)
        }),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    // Publish message
    ctx := context.Background()
    err = conn.PublishWithContext(ctx, []byte(`{"message":"Hello, RabbitMQ!"}`))
    if err != nil {
        log.Printf("Failed to publish: %v", err)
    }
}
```

### Consumer Example

```go
package main

import (
    "context"
    "log"
    
    "github.com/uoula/go-rabbitmq"
    amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
    // Create connection with options
    conn, err := rabbitmq.NewConnection("amqp://guest:guest@localhost:5672/",
        rabbitmq.WithCustomQueueDeclare(&rabbitmq.QueueDeclareOptions{
            Name:       "my-queue",
            Durable:    true,
            AutoDelete: false,
        }),
        rabbitmq.WithConsumerOptions(&rabbitmq.ConsumerOptions{
            Queue:         "my-queue",
            AutoAck:       false,
            PrefetchCount: 10,
        }),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    // Define message handler
    handler := func(ctx context.Context, msg amqp.Delivery) error {
        log.Printf("Received: %s", string(msg.Body))
        return nil // Returning nil will ack the message
    }

    // Start consuming
    err = conn.Consume(handler)
    if err != nil {
        log.Fatal(err)
    }

    // Block forever
    select {}
}
```

## API Reference

### NewConnection

```go
func NewConnection(url string, opts ...ConnectionOption) (*Connection, error)
```

Creates a new RabbitMQ connection with the given URL and options.

**Options:**
- `WithCustomQueueDeclare(*QueueDeclareOptions)` - Configure queue declaration
- `WithCustomPublishOptions(*PublishOptions)` - Configure publishing
- `WithConsumerOptions(*ConsumerOptions)` - Configure consumer
- `WithReconnectOptions(*ReconnectOptions)` - Configure reconnection behavior

### PublishWithContext

```go
func (c *Connection) PublishWithContext(ctx context.Context, data []byte) error
```

Publishes a message to the configured queue. Uses the routing key from `PublishOptions.Key`, or the queue name if not specified.

### Consume

```go
func (c *Connection) Consume(handler MessageHandler) error
```

Starts consuming messages with the provided handler function.

**Handler signature:**
```go
type MessageHandler func(context.Context, amqp.Delivery) error
```

- Return `nil` to acknowledge the message
- Return an error to nack and requeue the message

## Configuration

### QueueDeclareOptions

```go
type QueueDeclareOptions struct {
    Name       string
    Durable    bool      // Queue survives broker restart
    AutoDelete bool      // Queue deleted when no consumers
    Exclusive  bool      // Queue exclusive to this connection
    NoWait     bool      // Don't wait for server confirmation
    Args       amqp.Table // Additional arguments (e.g., x-queue-type)
}
```

**Default queue options:**
```go
opts := &rabbitmq.QueueDeclareOptions{
    Durable:    true,
    AutoDelete: false,
    Exclusive:  false,
    NoWait:     false,
    Args: amqp.Table{
        amqp.QueueTypeArg: amqp.QueueTypeQuorum, // Tries quorum, falls back to classic
    },
}
```

### PublishOptions

```go
type PublishOptions struct {
    Key       string // Routing key (defaults to queue name if empty)
    Mandatory bool   // Return unroutable messages
    Immediate bool   // Return messages that can't be immediately consumed
}
```

### ConsumerOptions

```go
type ConsumerOptions struct {
    Queue         string
    ConsumerTag   string // Auto-generated if empty
    AutoAck       bool   // Automatic acknowledgment
    Exclusive     bool   // Exclusive consumer
    PrefetchCount int    // Number of messages to prefetch
}
```

### ReconnectOptions

```go
type ReconnectOptions struct {
    MaxRetries        int           // 0 = infinite retries
    InitialBackoff    time.Duration // Initial backoff duration
    MaxBackoff        time.Duration // Maximum backoff duration
    BackoffMultiplier float64       // Backoff multiplier
}
```

**Defaults:**
```go
opts := &rabbitmq.ReconnectOptions{
    MaxRetries:        0,  // Infinite retries until Close()
    InitialBackoff:    1 * time.Second,
    MaxBackoff:        30 * time.Second,
    BackoffMultiplier: 2.0,
}
```

## Queue Declaration with Fallback

The library automatically tries to create a quorum queue first, and falls back to a classic queue if a 406 PRECONDITION_FAILED error is returned (e.g., when RabbitMQ version doesn't support quorum queues).

```go
opts := &rabbitmq.QueueDeclareOptions{
    Name:    "my-queue",
    Durable: true,
    Args: amqp.Table{
        amqp.QueueTypeArg: amqp.QueueTypeQuorum,
    },
}
```

**How it works:**
1. Attempts to declare a quorum queue
2. If 406 error is returned, automatically retries with classic queue
3. Logs the queue type that was successfully created

## Reconnection Behavior

The library automatically handles connection failures:

1. **Connection Monitoring**: Continuously monitors connection health
2. **Automatic Reconnection**: Reconnects with exponential backoff on failure
3. **Channel Recreation**: Automatically recreates channels for publishers and consumers
4. **Infinite Retries**: By default, retries indefinitely until `Close()` is called
5. **Jitter**: Adds random jitter to prevent thundering herd problem

## Testing

### Run Unit Tests

```bash
go test -v ./...
```

### Run Integration Tests

Integration tests require a running RabbitMQ instance:

```bash
# Start RabbitMQ
docker run -d --name rabbitmq-test -p 5672:5672 -p 15672:15672 rabbitmq:3-management

# Run integration tests
go test -v -tags=integration ./...

# Cleanup
docker stop rabbitmq-test && docker rm rabbitmq-test
```

## Examples

See the `examples/` directory for complete working examples:

- [`examples/publisher/main.go`](examples/publisher/main.go) - Publisher with periodic message sending
- [`examples/consumer/main.go`](examples/consumer/main.go) - Consumer with message processing

### Running Examples

```bash
# Terminal 1: Start consumer
cd examples/consumer
go run main.go

# Terminal 2: Start publisher
cd examples/publisher
go run main.go
```

## Best Practices

1. **Reuse Connections**: Create one connection per application
2. **Graceful Shutdown**: Always defer `Close()` calls
3. **Context Timeouts**: Use context with timeout for publishing
4. **Error Handling**: Always check errors from `PublishWithContext()`
5. **Message Handler**: Return `nil` to ack, return error to nack and requeue

## License

MIT License - see LICENSE file for details

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
