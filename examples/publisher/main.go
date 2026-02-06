package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/uoula/go-rabbitmq"
)

func main() {
	// Get RabbitMQ URL from environment or use default
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		url = "amqp://user:password@localhost:5672/"
	}

	// Create connection with custom options
	conn, err := rabbitmq.NewConnection(url,
		rabbitmq.WithCustomQueueDeclare(rabbitmq.DefaultQueueOptions("example-queue")),
		rabbitmq.WithCustomPublishOptions(&rabbitmq.PublishOptions{
			Key: "example-queue",
		}),
	)
	if err != nil {
		log.Fatalf("Failed to create connection: %v", err)
	}
	defer conn.Close()

	log.Println("Publisher started. Press Ctrl+C to stop.")

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Publish messages periodically
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	messageCount := 0

	for {
		select {
		case <-ticker.C:
			messageCount++

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

			body := []byte(`{"id":` + string(rune(messageCount+'0')) + `,"message":"Hello from publisher!"}`)

			err := conn.PublishWithContext(ctx, body)
			cancel()

			if err != nil {
				log.Printf("Failed to publish message: %v", err)
			} else {
				log.Printf("Published message #%d", messageCount)
			}

		case <-sigChan:
			log.Println("Shutting down publisher...")
			return
		}
	}
}
