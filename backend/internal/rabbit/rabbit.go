// Package rabbit owns the AMQP connection to RabbitMQ, the task queue. Domain
// modules never publish here directly: events go through the outbox, and the
// relay in worker publishes them (see ADR-004).
package rabbit

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Dial opens an AMQP connection.
func Dial(url string) (*amqp.Connection, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("amqp dial: %w", err)
	}
	return conn, nil
}
