module test_quorum

go 1.25.6

replace github.com/uoula/go-rabbitmq => ../

require (
	github.com/rabbitmq/amqp091-go v1.10.0
	github.com/uoula/go-rabbitmq v0.0.0-00010101000000-000000000000
)
