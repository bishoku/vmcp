package auditrabbitmq

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func env(t *testing.T) {
	t.Helper()
	t.Setenv("VMCP_AUDIT_RABBITMQ_URL", "amqp://audit:audit@localhost:5673/")
	t.Setenv("VMCP_AUDIT_RABBITMQ_EXCHANGE", "")
	t.Setenv("VMCP_AUDIT_RABBITMQ_ROUTING_KEY", "audit-test")
	t.Setenv("VMCP_AUDIT_EXPORT_ALLOW_PLAINTEXT", "true")
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8,::1/128")
	for _, k := range []string{"VMCP_EGRESS_ALLOW_HOSTS", "VMCP_AUDIT_EXPORT_TLS_CA_FILE", "VMCP_AUDIT_EXPORT_TLS_CERT_FILE", "VMCP_AUDIT_EXPORT_TLS_KEY_FILE"} {
		t.Setenv(k, "")
	}
}
func TestInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct{ key, value string }{{"VMCP_AUDIT_RABBITMQ_URL", "https://secret:password@broker"}, {"VMCP_AUDIT_RABBITMQ_URL", "amqps://host/#fragment"}, {"VMCP_AUDIT_RABBITMQ_ROUTING_KEY", ""}, {"VMCP_AUDIT_EXPORT_ALLOW_PLAINTEXT", ""}, {"VMCP_AUDIT_EXPORT_TLS_KEY_FILE", "/missing"}} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			env(t)
			t.Setenv(tc.key, tc.value)
			p, err := FromEnv()
			if p != nil {
				p.Close()
			}
			if err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
func TestBrokerConfirmReturnReconnectAndCancellation(t *testing.T) {
	uri := os.Getenv("VMCP_TEST_RABBITMQ_URL")
	if uri == "" {
		if os.Getenv("VMCP_REQUIRE_INTEGRATION") == "1" {
			t.Fatal("RabbitMQ integration URL required")
		}
		t.Skip("VMCP_TEST_RABBITMQ_URL not set")
	}
	env(t)
	t.Setenv("VMCP_AUDIT_RABBITMQ_URL", uri)
	conn, err := amqp.Dial(uri)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	queue, err := ch.QueueDeclare("", true, true, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ch.QueueDelete(queue.Name, false, false, false)
	t.Setenv("VMCP_AUDIT_RABBITMQ_ROUTING_KEY", queue.Name)
	p, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	raw := []byte(`{"event_id":"event-1"}`)
	for i := 0; i < 2; i++ {
		if err = p.Publish(ctx, "event-1", raw); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		m, ok, err := ch.Get(queue.Name, true)
		if err != nil || !ok {
			t.Fatal("missing confirmed message", err)
		}
		if m.MessageId != "event-1" || string(m.Body) != string(raw) || m.DeliveryMode != amqp.Persistent {
			t.Fatal("identity/persistence lost")
		}
	}
	p.socket.Close()
	p.conn.CloseDeadline(time.Now().Add(time.Second))
	if err = p.Publish(ctx, "after-reconnect", raw); err != nil {
		t.Fatal(err)
	}
	p.routingKey = "missing-" + queue.Name
	if p.Publish(ctx, "unroutable", raw) == nil {
		t.Fatal("unroutable event acknowledged")
	}
	p.routingKey = queue.Name
	if err = p.Publish(ctx, "after-return", raw); err != nil {
		t.Fatal(err)
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if p.Publish(cancelled, "cancelled", raw) == nil {
		t.Fatal("cancelled publish acknowledged")
	}
	p.exchange = "missing-" + queue.Name
	if p.Publish(ctx, "missing-exchange", raw) == nil {
		t.Fatal("closed channel acknowledged")
	}
}

func TestHandshakeCancellation(t *testing.T) {
	env(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	release := make(chan struct{})
	defer close(release)
	go func() {
		socket, err := listener.Accept()
		if err == nil {
			defer socket.Close()
			<-release
		}
	}()
	t.Setenv("VMCP_AUDIT_RABBITMQ_URL", "amqp://audit:audit@"+listener.Addr().String()+"/")
	p, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	if p.Publish(ctx, "test", []byte(`{}`)) == nil {
		t.Fatal("silent peer acknowledged")
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("AMQP handshake ignored cancellation")
	}
}
