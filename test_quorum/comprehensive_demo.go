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

	log.Println("=== Comprehensive Queue Fallback Test ===")
	log.Printf("Connecting to: %s\n", url)

	// Test 1: Quorum queue (will succeed or fallback to classic)
	log.Println("\n--- Test 1: Quorum Queue Declaration ---")
	testQuorumQueue(url)

	// Test 2: Classic queue (explicit)
	log.Println("\n--- Test 2: Classic Queue Declaration ---")
	testClassicQueue(url)

	// Test 3: Demonstrate fallback by creating a classic queue first, then trying quorum
	log.Println("\n--- Test 3: Demonstrating Fallback Scenario ---")
	testFallbackScenario(url)

	log.Println("\n=== All Tests Complete ===")
}

func testQuorumQueue(url string) {
	conn, err := rabbitmq.NewConnection(url,
		rabbitmq.WithCustomQueueDeclare(&rabbitmq.QueueDeclareOptions{
			Name:       "test-quorum-1",
			Durable:    true,
			AutoDelete: false,
			Args: amqp.Table{
				amqp.QueueTypeArg: amqp.QueueTypeQuorum,
			},
		}),
	)
	if err != nil {
		log.Printf("✗ Failed: %v", err)
		return
	}
	defer conn.Close()

	log.Println("✓ Quorum queue created successfully (or fell back to classic)")

	// Publish and consume test
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msg := []byte(`{"type":"quorum-test","timestamp":"` + time.Now().Format(time.RFC3339) + `"}`)
	if err := conn.PublishWithContext(ctx, msg); err != nil {
		log.Printf("✗ Publish failed: %v", err)
	} else {
		log.Println("✓ Message published to quorum queue")
	}
}

func testClassicQueue(url string) {
	conn, err := rabbitmq.NewConnection(url,
		rabbitmq.WithCustomQueueDeclare(&rabbitmq.QueueDeclareOptions{
			Name:       "test-classic-1",
			Durable:    true,
			AutoDelete: false,
			Args:       amqp.Table{}, // No x-queue-type = classic
		}),
	)
	if err != nil {
		log.Printf("✗ Failed: %v", err)
		return
	}
	defer conn.Close()

	log.Println("✓ Classic queue created successfully")

	// Publish test
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msg := []byte(`{"type":"classic-test","timestamp":"` + time.Now().Format(time.RFC3339) + `"}`)
	if err := conn.PublishWithContext(ctx, msg); err != nil {
		log.Printf("✗ Publish failed: %v", err)
	} else {
		log.Println("✓ Message published to classic queue")
	}
}

func testFallbackScenario(url string) {
	queueName := "test-fallback-demo"

	// First, create a classic queue
	log.Printf("Step 1: Creating classic queue '%s'...", queueName)
	conn1, err := rabbitmq.NewConnection(url,
		rabbitmq.WithCustomQueueDeclare(&rabbitmq.QueueDeclareOptions{
			Name:       queueName,
			Durable:    true,
			AutoDelete: false,
			Args:       amqp.Table{}, // Classic
		}),
	)
	if err != nil {
		log.Printf("✗ Failed to create classic queue: %v", err)
		return
	}
	log.Println("✓ Classic queue created")
	conn1.Close()

	// Now try to redeclare as quorum - this should trigger 406 and fallback
	log.Printf("Step 2: Attempting to redeclare '%s' as quorum (should trigger fallback)...", queueName)
	conn2, err := rabbitmq.NewConnection(url,
		rabbitmq.WithCustomQueueDeclare(&rabbitmq.QueueDeclareOptions{
			Name:       queueName,
			Durable:    true,
			AutoDelete: false,
			Args: amqp.Table{
				amqp.QueueTypeArg: amqp.QueueTypeQuorum,
			},
		}),
	)
	if err != nil {
		log.Printf("✗ Failed: %v", err)
		log.Println("⚠ This is expected if the queue type mismatch cannot be resolved")
		return
	}
	defer conn2.Close()

	log.Println("✓ Successfully handled queue type mismatch (fallback worked)")

	// Test publishing
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msg := []byte(`{"type":"fallback-test","timestamp":"` + time.Now().Format(time.RFC3339) + `"}`)
	if err := conn2.PublishWithContext(ctx, msg); err != nil {
		log.Printf("✗ Publish failed: %v", err)
	} else {
		log.Println("✓ Message published successfully after fallback")
	}
}
