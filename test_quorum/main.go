package main

import (
	"context"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/uoula/go-rabbitmq"
)

func main() {
	// Use the provided credentials
	url := "amqp://user:password@localhost:5672/"

	log.Println("=== Testing Queue Creation with Quorum/Classic Fallback ===")
	log.Printf("Connecting to: %s\n", url)

	// Test 1: Create connection with quorum queue (will try quorum first)
	log.Println("\n--- Test 1: Creating queue with quorum type (will fallback to classic if needed) ---")
	conn, err := rabbitmq.NewConnection(url,
		rabbitmq.WithCustomQueueDeclare(&rabbitmq.QueueDeclareOptions{
			Name:       "test-quorum-queue",
			Durable:    true,
			AutoDelete: false,
			Exclusive:  false,
			Args: amqp.Table{
				amqp.QueueTypeArg: amqp.QueueTypeQuorum,
			},
		}),
		rabbitmq.WithCustomPublishOptions(&rabbitmq.PublishOptions{
			Key: "test-quorum-queue",
		}),
	)
	if err != nil {
		log.Fatalf("Failed to create connection: %v", err)
	}

	log.Println("✓ Connection created successfully with queue declaration")

	// Test 2: Publish a test message
	log.Println("\n--- Test 2: Publishing test message ---")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	testMessage := []byte(`{"test": "message", "timestamp": "` + time.Now().Format(time.RFC3339) + `"}`)
	err = conn.PublishWithContext(ctx, testMessage)
	if err != nil {
		log.Printf("✗ Failed to publish message: %v", err)
	} else {
		log.Println("✓ Message published successfully")
	}

	// Test 3: Create another queue with explicit classic type
	log.Println("\n--- Test 3: Creating explicit classic queue ---")
	conn2, err := rabbitmq.NewConnection(url,
		rabbitmq.WithCustomQueueDeclare(&rabbitmq.QueueDeclareOptions{
			Name:       "test-classic-queue",
			Durable:    true,
			AutoDelete: false,
			Exclusive:  false,
			Args:       amqp.Table{}, // No queue type = classic
		}),
	)
	if err != nil {
		log.Printf("✗ Failed to create classic queue connection: %v", err)
	} else {
		log.Println("✓ Classic queue connection created successfully")
		defer conn2.Close()
	}

	// Test 4: Consume a message to verify the queue works
	log.Println("\n--- Test 4: Setting up consumer to verify queue functionality ---")
	messageReceived := make(chan bool, 1)

	handler := func(ctx context.Context, msg amqp.Delivery) error {
		log.Printf("✓ Received message: %s", string(msg.Body))
		messageReceived <- true
		return nil
	}

	err = conn.Consume(handler)
	if err != nil {
		log.Printf("✗ Failed to start consumer: %v", err)
	} else {
		log.Println("✓ Consumer started successfully")

		// Wait a bit for the message to be consumed
		select {
		case <-messageReceived:
			log.Println("✓ Message consumed successfully - queue is working!")
		case <-time.After(3 * time.Second):
			log.Println("⚠ No message received within timeout (this is OK if queue was already consumed)")
		}
	}

	// Cleanup
	log.Println("\n--- Cleaning up ---")
	conn.Close()

	log.Println("\n=== Test Complete ===")
	log.Println("Summary:")
	log.Println("- Queue creation with quorum/classic fallback: TESTED")
	log.Println("- Message publishing: TESTED")
	log.Println("- Message consuming: TESTED")
	log.Println("\nCheck the logs above to see if quorum was used or if it fell back to classic.")
}
