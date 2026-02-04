package rabbitmq

import (
	"fmt"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

// QueueDeclareOptions holds options for queue declaration
type QueueDeclareOptions struct {
	// Name is the queue name
	Name string

	/*
		Durable and Non-Auto-Deleted queues will survive server restarts and remain
		when there are no remaining consumers or bindings.  Persistent publishings will
		be restored in this queue on server restart.  These queues are only able to be
		bound to durable exchanges.
	*/
	Durable bool

	/*
		Non-Durable and Auto-Deleted queues will not be redeclared on server restart
		and will be deleted by the server after a short time when the last consumer is
		canceled or the last consumer's channel is closed.  Queues with this lifetime
		can also be deleted normally with QueueDelete.  These durable queues can only
		be bound to non-durable exchanges.

		Non-Durable and Non-Auto-Deleted queues will remain declared as long as the
		server is running regardless of how many consumers.  This lifetime is useful
		for temporary topologies that may have long delays between consumer activity.
		These queues can only be bound to non-durable exchanges.

		Durable and Auto-Deleted queues will be restored on server restart, but without
		active consumers will not survive and be removed.  This Lifetime is unlikely
		to be useful.
	*/
	AutoDelete bool

	/*
		Exclusive queues are only accessible by the connection that declares them and
		will be deleted when the connection closes.  Channels on other connections
		will receive an error when attempting  to declare, bind, consume, purge or
		delete a queue with the same name.
	*/
	Exclusive bool

	/*
		When noWait is true, the queue will assume to be declared on the server.  A
		channel exception will arrive if the conditions are met for existing queues
		or attempting to modify an existing queue from a different connection.
	*/
	NoWait bool

	Args amqp.Table
}

// DefaultQueueOptions returns default queue declaration options
func DefaultQueueOptions(name string) *QueueDeclareOptions {
	return &QueueDeclareOptions{
		Name:       name,
		Durable:    true,
		AutoDelete: false,
		Exclusive:  false,
		NoWait:     false,
		Args: amqp.Table{
			amqp.QueueTypeArg: amqp.QueueTypeQuorum,
		},
	}
}

// DeclareQuorumQueue declares a queue, attempting quorum type first,
// falling back to classic if 406 PRECONDITION_FAILED is returned
func DeclareQuorumQueue(ch *amqp.Channel, opts *QueueDeclareOptions) (amqp.Queue, error) {
	if opts == nil {
		return amqp.Queue{}, fmt.Errorf("queue options cannot be nil")
	}

	// First attempt: try with provided args (may include quorum type)
	queue, err := ch.QueueDeclare(
		opts.Name,
		opts.Durable,
		opts.AutoDelete,
		opts.Exclusive,
		opts.NoWait,
		opts.Args,
	)

	if err != nil {
		// Check if it's a 406 PRECONDITION_FAILED error
		if amqpErr, ok := err.(*amqp.Error); ok && amqpErr.Code == 406 {
			log.Printf("Quorum queue declaration failed (406), falling back to classic queue for: %s", opts.Name)

			// Second attempt: try classic queue (remove x-queue-type)
			classicArgs := amqp.Table{}
			if opts.Args != nil {
				for k, v := range opts.Args {
					// Don't copy x-queue-type
					if k != amqp.QueueTypeArg {
						classicArgs[k] = v
					}
				}
			}

			queue, err = ch.QueueDeclare(
				opts.Name,
				opts.Durable,
				opts.AutoDelete,
				opts.Exclusive,
				opts.NoWait,
				classicArgs,
			)

			if err != nil {
				return amqp.Queue{}, fmt.Errorf("failed to declare classic queue: %w", err)
			}

			log.Printf("Successfully declared classic queue: %s", opts.Name)
			return queue, nil
		}

		return amqp.Queue{}, fmt.Errorf("failed to declare queue: %w", err)
	}

	log.Printf("Successfully declared queue: %s", opts.Name)
	return queue, nil
}
