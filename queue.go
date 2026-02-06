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

// DeclareQuorumQueue declares a queue with the given options
// If a 406 error occurs, the caller should handle channel recreation and fallback
func DeclareQuorumQueue(ch *amqp.Channel, opts *QueueDeclareOptions) (amqp.Queue, error) {
	if opts == nil {
		return amqp.Queue{}, fmt.Errorf("queue options cannot be nil")
	}

	queue, err := ch.QueueDeclare(
		opts.Name,
		opts.Durable,
		opts.AutoDelete,
		opts.Exclusive,
		opts.NoWait,
		opts.Args,
	)

	if err != nil {
		// Return the error as-is, let caller handle 406 errors
		return amqp.Queue{}, err
	}

	log.Printf("Successfully declared queue: %s", opts.Name)
	return queue, nil
}
