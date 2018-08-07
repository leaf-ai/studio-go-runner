package runner

// This contains the implementation of a RabbitMQ (rmq) client that will
// be used to retrieve work from RMQ and to query RMQ for extant queues
// within an StudioML Exchange

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
	"github.com/streadway/amqp"

	rh "github.com/michaelklishin/rabbit-hole"
)

func init() {
}

type RabbitMQ struct {
	url       *url.URL // amqp URL to be used for the rmq Server
	exchange  string
	mgmt      *url.URL        // URL for the management interface on the rmq
	user      string          // user name for the management interface on rmq
	pass      string          // password for the management interface on rmq
	transport *http.Transport // Custom transport to allow for connections to be actively closed
}

func NewRabbitMQ(uri string, queue string) (rmq *RabbitMQ, err errors.Error) {

	rmq = &RabbitMQ{
		// "amqp://guest:guest@localhost:5672/%2F?connection_attempts=50",
		// "http://127.0.0.1:15672",
		exchange: "StudioML.topic",
		user:     "guest",
		pass:     "guest",
	}

	ampq, errGo := url.Parse(uri)
	if errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("uri", uri)
	}
	rmq.url = ampq
	hp := strings.Split(ampq.Host, ":")
	userPass := strings.SplitN(ampq.User.String(), ":", 2)
	if len(userPass) != 2 {
		return nil, errors.New("Username password missing or malformed").With("stack", stack.Trace().TrimRuntime()).With("uri", ampq.String())
	}
	rmq.user = userPass[0]
	rmq.pass = userPass[1]
	rmq.mgmt = &url.URL{
		Scheme: "http",
		User:   url.UserPassword(userPass[0], userPass[1]),
		Host:   fmt.Sprintf("%s:%d", hp[0], 15672),
	}
	return rmq, nil
}

func (rmq *RabbitMQ) attachQ() (conn *amqp.Connection, ch *amqp.Channel, err errors.Error) {

	conn, errGo := amqp.Dial(rmq.url.String())
	if errGo != nil {
		return nil, nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("uri", rmq.url)
	}

	if ch, errGo = conn.Channel(); errGo != nil {
		return nil, nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("uri", rmq.url)
	}

	if errGo := ch.ExchangeDeclare(rmq.exchange, "topic", true, true, false, false, nil); errGo != nil {
		return nil, nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("uri", rmq.url).With("exchange", rmq.exchange)
	}
	return conn, ch, nil
}

func (rmq *RabbitMQ) attachMgmt(timeout time.Duration) (mgmt *rh.Client, err errors.Error) {
	user := rmq.mgmt.User.Username()
	pass, _ := rmq.mgmt.User.Password()

	mgmt, errGo := rh.NewClient(rmq.mgmt.String(), user, pass)
	if errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("user", user).With("uri", rmq.mgmt).With("exchange", rmq.exchange)
	}

	if rmq.transport == nil {
		rmq.transport = &http.Transport{
			MaxIdleConns:    1,
			IdleConnTimeout: timeout,
		}
	}
	mgmt.SetTransport(rmq.transport)

	return mgmt, nil
}

// Refresh will examine the RMQ exchange a extract a list of the queues that relate to
// StudioML work from the rmq exchange
func (rmq *RabbitMQ) Refresh(matcher *regexp.Regexp, timeout time.Duration) (known map[string]interface{}, err errors.Error) {
	known = map[string]interface{}{}

	mgmt, err := rmq.attachMgmt(timeout)
	if err != nil {
		return known, err
	}
	defer func() {
		rmq.transport.CloseIdleConnections()
	}()

	binds, errGo := mgmt.ListBindings()
	if errGo != nil {
		return known, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("uri", rmq.mgmt)
	}

	for _, b := range binds {
		if b.Source == "StudioML.topic" && strings.HasPrefix(b.RoutingKey, "StudioML.") {
			// Make sure any retrieved Q names match the caller supplied regular expression
			if matcher != nil {
				if !matcher.MatchString(b.Destination) {
					continue
				}
			}
			queue := fmt.Sprintf("%s?%s", url.PathEscape(b.Vhost), url.PathEscape(b.Destination))
			known[queue] = b.Vhost
		}
	}

	return known, nil
}

func (rmq *RabbitMQ) GetKnown(matcher *regexp.Regexp, timeout time.Duration) (found map[string]string, err errors.Error) {
	found = map[string]string{}

	keyPrefix, errGo := url.PathUnescape(rmq.url.String())
	if errGo != nil {
		return nil, errors.Wrap(errGo).With("url", rmq.url.String()).With("stack", stack.Trace().TrimRuntime())
	}
	keyPrefix = strings.TrimRight(keyPrefix, "?")

	known, err := rmq.Refresh(matcher, timeout)
	if err != nil {
		return nil, err
	} else {
		for hostQueue := range known {
			splits := strings.SplitN(hostQueue, "?", 2)
			if len(splits) != 2 {
				return nil, errors.New("missing seperator in hostQueue").With("hostQueue", hostQueue).With("stack", stack.Trace().TrimRuntime())
			}
			dest, errGo := url.PathUnescape(splits[1])
			if errGo != nil {
				return nil, errors.Wrap(errGo).With("hostQueue", hostQueue).With("stack", stack.Trace().TrimRuntime())
			}
			dest = strings.TrimLeft(dest, "?")
			found[keyPrefix+"?"+dest] = dest
		}
	}
	return found, nil
}

func (rmq *RabbitMQ) Exists(ctx context.Context, subscription string) (exists bool, err errors.Error) {
	destHost := strings.Split(subscription, "?")
	if len(destHost) != 2 {
		return false, errors.New("subscription supplied was not question-mark seperated").With("stack", stack.Trace().TrimRuntime()).With("subscription", subscription)
	}

	vhost, errGo := url.PathUnescape(destHost[0])
	if errGo != nil {
		return false, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("subscription", subscription).With("vhost", destHost[0])
	}
	queue, errGo := url.PathUnescape(destHost[1])
	if errGo != nil {
		return false, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("subscription", subscription).With("queue", destHost[1])
	}

	mgmt, err := rmq.attachMgmt(15 * time.Second)
	if err != nil {
		return false, err
	}
	defer func() {
		rmq.transport.CloseIdleConnections()
	}()

	if _, errGo = mgmt.GetQueue(vhost, queue); errGo != nil {
		return false, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("uri", rmq.mgmt)
	}

	return true, nil
}

func (rmq *RabbitMQ) Work(ctx context.Context, qTimeout time.Duration,
	subscription string, handler MsgHandler) (msgCnt uint64, resource *Resource, err errors.Error) {

	splits := strings.SplitN(subscription, "?", 2)
	if len(splits) != 2 {
		return 0, nil, errors.New("malformed rmq subscription").With("stack", stack.Trace().TrimRuntime()).With("subscription", subscription)
	}

	conn, ch, err := rmq.attachQ()
	if err != nil {
		return 0, nil, err
	}
	defer func() {
		ch.Close()
		conn.Close()
	}()

	queue, errGo := url.PathUnescape(splits[1])
	if errGo != nil {
		return 0, nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("subscription", subscription)
	}
	queue = strings.Trim(queue, "/")

	msg, ok, errGo := ch.Get(queue, false)
	if errGo != nil {
		return 0, nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("queue", queue)
	}
	if !ok {
		return 0, nil, nil
	}

	rsc, ack := handler(ctx, rmq.url.String(), rmq.url.String(), "", msg.Body)

	if ack {
		resource = rsc
		if errGo := msg.Ack(false); errGo != nil {
			return 0, nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("subscription", subscription)
		}
	} else {
		msg.Nack(false, true)
	}

	return 1, resource, nil
}
