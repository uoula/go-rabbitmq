package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/uoula/go-rabbitmq"
)

func main() {
	// Get RabbitMQ URL from environment or use default
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		url = "amqp://guest:guest@localhost:5672/"
	}

	// Create connection with custom options
	conn, err := rabbitmq.NewConnection(url,
		rabbitmq.WithCustomQueueDeclare(&rabbitmq.QueueDeclareOptions{
			Name:       "example-queue",
			Durable:    true,
			AutoDelete: false,
		}),
		rabbitmq.WithConsumerOptions(&rabbitmq.ConsumerOptions{
			Queue:         "example-queue",
			AutoAck:       false,
			PrefetchCount: 5,
		}),
	)
	if err != nil {
		log.Fatalf("Failed to create connection: %v", err)
	}
	defer conn.Close()

	// Define message handler
	messageHandler := func(ctx context.Context, msg amqp.Delivery) error {
		log.Printf("Received message: %s", string(msg.Body))
		log.Printf("  Headers: %v", msg.Headers)
		log.Printf("  Timestamp: %v", msg.Timestamp)

		// Simulate processing time
		time.Sleep(500 * time.Millisecond)

		log.Printf("Processed message successfully")
		return nil
	}

	// Start consuming
	err = conn.Consume(messageHandler)
	if err != nil {
		log.Fatalf("Failed to start consuming: %v", err)
	}

	log.Println("Consumer started. Waiting for messages. Press Ctrl+C to stop.")

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	<-sigChan
	log.Println("Shutting down consumer...")

	// conn.Close() will wait for in-flight messages to complete
	log.Println("Waiting for in-flight messages to complete...")
}
