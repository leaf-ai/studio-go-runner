package runner

// This contains the implementation of a RabbitMQ (rmq) client that will
// be used to retrieve work from RMQ and to query RMQ for extant queues
// within an StudioML Exchange

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
	"github.com/rs/xid"

	rh "github.com/michaelklishin/rabbit-hole"
	"github.com/streadway/amqp"
)

// RabbitMQ encapsulated the configuration and extant extant client for a
// queue server
//
type RabbitMQ struct {
	url       *url.URL // amqp URL to be used for the rmq Server
	safeURL   string   // A URL stripped of the user name and password, making it safe for logging etc
	exchange  string
	mgmt      *url.URL        // URL for the management interface on the rmq
	user      string          // user name for the management interface on rmq
	pass      string          // password for the management interface on rmq
	transport *http.Transport // Custom transport to allow for connections to be actively closed
}

// NewRabbitMQ takes the uri identifing a server and will configure the client
// data structure needed to call methods against the server
//
// The order of these two parameters needs to reflect key, value pair that
// the GetKnown function returns
//
func NewRabbitMQ(uri string, authURI string) (rmq *RabbitMQ, err errors.Error) {

	rmq = &RabbitMQ{
		// "amqp://guest:guest@localhost:5672/%2F?connection_attempts=50",
		// "http://127.0.0.1:15672",
		exchange: "StudioML.topic",
		user:     "guest",
		pass:     "guest",
	}

	ampq, errGo := url.Parse(os.ExpandEnv(authURI))
	if errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("uri", os.ExpandEnv(uri))
	}
	rmq.url = ampq
	rmq.safeURL = strings.Replace(os.ExpandEnv(uri), ampq.User.String()+"@", "", 1)

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
		return nil, nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("uri", rmq.safeURL)
	}

	if ch, errGo = conn.Channel(); errGo != nil {
		return nil, nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("uri", rmq.safeURL)
	}

	if errGo := ch.ExchangeDeclare(rmq.exchange, "topic", true, true, false, false, nil); errGo != nil {
		return nil, nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("uri", rmq.safeURL).With("exchange", rmq.exchange)
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

// GetKnown will connect to the rabbitMQ server identified in the receiver, rmq, and will
// query it for any queues that match the matcher regular expression
//
func (rmq *RabbitMQ) GetKnown(matcher *regexp.Regexp, timeout time.Duration) (found map[string]string, err errors.Error) {
	known, err := rmq.Refresh(matcher, timeout)
	if err != nil {
		return nil, err
	}

	found = map[string]string{}

	for hostQueue := range known {
		found[rmq.safeURL+"?"+hostQueue] = rmq.url.String()
	}
	return found, nil
}

// Exists will connect to the rabbitMQ server identified in the receiver, rmq, and will
// query it to see if the queue identified by the studio go runner subscription exists
//
func (rmq *RabbitMQ) Exists(ctx context.Context, subscription string) (exists bool, err errors.Error) {
	destHost := strings.Split(subscription, "?")
	if len(destHost) != 2 {
		return false, errors.New("subscription supplied was not question-mark separated").With("stack", stack.Trace().TrimRuntime()).With("subscription", subscription)
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

// Work will connect to the rabbitMQ server identified in the receiver, rmq, and will see if any work
// can be found on the queue identified by the go runner subscription and present work
// to the handler for processing
//
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

	//rsc, ack := handler(ctx, rmq.url.String(), rmq.url.String(), "", msg.Body)
	rsc, ack := handler(ctx, rmq.safeURL, rmq.safeURL, "", msg.Body)

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

// This file contains the implementation of a test subsystem
// for deploying rabbitMQ in test scenarios where it
// has been installed for the purposes of running end-to-end
// tests related to queue handling and state management

var (
	testQErr = errors.New("uninitialized").With("stack", stack.Trace().TrimRuntime())
	qCheck   sync.Once
)

// PingRMQServer is used to validate the a RabbitMQ server is alive and active on the administration port.
//
// amqpURL is the standard client amqp uri supplied by a caller. amqpURL will be parsed and converted into
// the administration endpoint and then tested.
//
func PingRMQServer(amqpURL string) (err errors.Error) {

	qCheck.Do(func() {

		if len(amqpURL) == 0 {
			testQErr = errors.New("amqpURL was not specified on the command line, or as an env var, cannot start rabbitMQ").With("stack", stack.Trace().TrimRuntime())
			return
		}

		q := os.ExpandEnv(amqpURL)

		uri, errGo := amqp.ParseURI(q)
		if errGo != nil {
			testQErr = errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			return
		}
		uri.Port += 10000

		// Start by making sure that when things were started we saw a rabbitMQ configured
		// on the localhost.  If so then check that the rabbitMQ started automatically as a result of
		// the Dockerfile_full setup
		//
		rmqc, errGo := rh.NewClient("http://"+uri.Host+":"+strconv.Itoa(uri.Port), uri.Username, uri.Password)
		if errGo != nil {
			testQErr = errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			return
		}

		rmqc.SetTransport(&http.Transport{
			ResponseHeaderTimeout: time.Duration(15 * time.Second),
		})
		rmqc.SetTimeout(time.Duration(15 * time.Second))

		// declares a queue
		if _, errGo = rmqc.DeclareQueue("/", "rmq_runner_test_"+xid.New().String(), rh.QueueSettings{Durable: false}); errGo != nil {
			testQErr = errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			return
		}

		testQErr = nil
	})

	return testQErr
}
