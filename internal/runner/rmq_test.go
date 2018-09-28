package runner

import (
	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
	"github.com/streadway/amqp"
)

// This file contains methods that extend the RabbitMQ features to send studioml
// experiments that are needed for testing

// QueueDeclare is a shim method for creating a queue within the rabbitMQ
// server defined by the receiver
//
func (rmq *RabbitMQ) QueueDeclare(qName string) (err errors.Error) {
	conn, ch, err := rmq.attachQ()
	if err != nil {
		return err
	}
	defer func() {
		ch.Close()
		conn.Close()
	}()

	_, errGo := ch.QueueDeclare(
		qName, // name
		false, // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("qName", qName).With("uri", rmq.mgmt).With("exchange", rmq.exchange)
	}
	return nil
}

// Publish is a shim method for tests to use for sending requeues to a queue
//
func (rmq *RabbitMQ) Publish(routingKey string, contentType string, msg []byte) (err errors.Error) {
	conn, ch, err := rmq.attachQ()
	if err != nil {
		return err
	}
	defer func() {
		ch.Close()
		conn.Close()
	}()

	errGo := ch.Publish(
		rmq.exchange, // exchange
		routingKey,   // routing key
		false,        // mandatory
		false,        // immediate
		amqp.Publishing{
			ContentType: contentType,
			Body:        msg,
		})
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("routingKey", routingKey).With("uri", rmq.mgmt).With("exchange", rmq.exchange)
	}
	return nil
}
