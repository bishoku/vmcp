// Package auditrabbitmq is imported only by the RabbitMQ audit exporter.
package auditrabbitmq

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/bishoku/vmcp/core/internal/auditexport"
	"github.com/bishoku/vmcp/core/internal/netpolicy"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Publisher struct {
	mu                        sync.Mutex
	uri, exchange, routingKey string
	tls                       *tls.Config
	policy                    *netpolicy.Policy
	conn                      *amqp.Connection
	channel                   *amqp.Channel
	socket                    net.Conn
	returns                   chan amqp.Return
}

func FromEnv() (*Publisher, error) {
	uri := os.Getenv("VMCP_AUDIT_RABBITMQ_URL")
	u, err := url.Parse(uri)
	if err != nil || u.Hostname() == "" || u.Fragment != "" || u.RawQuery != "" || (u.Scheme != "amqps" && !(u.Scheme == "amqp" && os.Getenv("VMCP_AUDIT_EXPORT_ALLOW_PLAINTEXT") == "true")) {
		return nil, errors.New("RabbitMQ requires an AMQPS URL (plaintext requires explicit development opt-in)")
	}
	if _, err = amqp.ParseURI(uri); err != nil {
		return nil, errors.New("invalid RabbitMQ URL")
	}
	exchange, key := os.Getenv("VMCP_AUDIT_RABBITMQ_EXCHANGE"), os.Getenv("VMCP_AUDIT_RABBITMQ_ROUTING_KEY")
	if len(exchange) > 255 || len(key) > 255 || key == "" {
		return nil, errors.New("invalid RabbitMQ exchange or routing key")
	}
	cfg, err := auditexport.TLSConfig()
	if err != nil {
		return nil, err
	}
	policy, err := netpolicy.FromEnv()
	if err != nil {
		return nil, errors.New("invalid audit egress policy")
	}
	return &Publisher{uri: uri, exchange: exchange, routingKey: key, tls: cfg, policy: policy}, nil
}

// reset closes the underlying socket directly so shutdown never waits for an AMQP close handshake.
func (p *Publisher) reset() {
	if p.socket != nil {
		p.socket.Close()
	}
	p.conn = nil
	p.channel = nil
	p.socket = nil
	p.returns = nil
}
func (p *Publisher) connect(ctx context.Context) error {
	if p.conn != nil && !p.conn.IsClosed() && p.channel != nil && !p.channel.IsClosed() {
		return nil
	}
	p.reset()
	// AMQP connection/channel RPCs have no context API. Closing the local socket
	// on cancellation bounds those waits even if heartbeat logic changes deadlines.
	var stop func() bool
	defer func() {
		if stop != nil {
			stop()
		}
	}()
	conn, err := amqp.DialConfig(p.uri, amqp.Config{TLSClientConfig: p.tls.Clone(), Heartbeat: 10 * time.Second, Dial: func(network, address string) (net.Conn, error) {
		socket, err := p.policy.DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		p.socket = socket
		stop = context.AfterFunc(ctx, func() { socket.Close() })
		if deadline, ok := ctx.Deadline(); ok {
			socket.SetDeadline(deadline)
		}
		return socket, nil
	}})
	if err != nil {
		p.reset()
		return err
	}
	p.conn = conn
	if deadline, ok := ctx.Deadline(); ok {
		p.socket.SetDeadline(deadline)
	}
	channel, err := conn.Channel()
	if err != nil {
		p.reset()
		return err
	}
	p.channel = channel
	if err = channel.Confirm(false); err != nil {
		p.reset()
		return err
	}
	p.returns = channel.NotifyReturn(make(chan amqp.Return, 1))
	return nil
}
func (p *Publisher) Publish(ctx context.Context, id string, raw []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := bounded.Err(); err != nil {
		return err
	}
	if err := p.connect(bounded); err != nil {
		return errors.New("RabbitMQ connection unavailable")
	}
	socket := p.socket
	stop := context.AfterFunc(bounded, func() { socket.Close() })
	defer stop()
	deadline, _ := bounded.Deadline()
	p.socket.SetDeadline(deadline)
	// amqp091-go's publish context does not interrupt socket writes: deadline bounds those too.
	confirmation, err := p.channel.PublishWithDeferredConfirmWithContext(bounded, p.exchange, p.routingKey, true, false, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, MessageId: id, Type: "vmcp.audit.event", Body: raw})
	if err != nil {
		p.reset()
		return errors.New("RabbitMQ audit publish failed")
	}
	accepted, err := confirmation.WaitContext(bounded)
	// The library enqueues basic.return before handling the corresponding basic.ack.
	returned := false
	select {
	case _, ok := <-p.returns:
		returned = ok
	default:
	}
	if err != nil || !accepted || returned {
		p.reset()
		return errors.New("RabbitMQ audit delivery not confirmed or unroutable")
	}
	p.socket.SetDeadline(time.Time{})
	return nil
}
func (p *Publisher) Close() { p.mu.Lock(); defer p.mu.Unlock(); p.reset() }
