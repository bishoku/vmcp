package auditkafka

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

func env(t *testing.T) {
	t.Helper()
	for _, k := range []string{"VMCP_AUDIT_KAFKA_USERNAME", "VMCP_AUDIT_KAFKA_PASSWORD", "VMCP_AUDIT_KAFKA_SASL", "VMCP_AUDIT_EXPORT_TLS_CA_FILE", "VMCP_AUDIT_EXPORT_TLS_CERT_FILE", "VMCP_AUDIT_EXPORT_TLS_KEY_FILE", "VMCP_EGRESS_ALLOW_HOSTS"} {
		t.Setenv(k, "")
	}
	t.Setenv("VMCP_AUDIT_KAFKA_BROKERS", "localhost:19092")
	t.Setenv("VMCP_AUDIT_KAFKA_TOPIC", "audit-test")
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8,::1/128")
	t.Setenv("VMCP_AUDIT_EXPORT_ALLOW_PLAINTEXT", "true")
}
func TestInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct{ key, value string }{{"VMCP_AUDIT_KAFKA_BROKERS", "http://broker"}, {"VMCP_AUDIT_KAFKA_TOPIC", ".."}, {"VMCP_AUDIT_KAFKA_SASL", "unsupported"}, {"VMCP_AUDIT_KAFKA_USERNAME", "user"}, {"VMCP_AUDIT_KAFKA_SASL", "PLAIN"}, {"VMCP_AUDIT_EXPORT_TLS_CA_FILE", "/missing"}} {
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
func TestBrokerPublishDuplicateAndCancellation(t *testing.T) {
	brokers := os.Getenv("VMCP_TEST_KAFKA_BROKERS")
	if brokers == "" {
		if os.Getenv("VMCP_REQUIRE_INTEGRATION") == "1" {
			t.Fatal("Kafka integration URL required")
		}
		t.Skip("VMCP_TEST_KAFKA_BROKERS not set")
	}
	env(t)
	t.Setenv("VMCP_AUDIT_KAFKA_BROKERS", brokers)
	topic := "audit-test-" + time.Now().Format("150405.000000000")
	t.Setenv("VMCP_AUDIT_KAFKA_TOPIC", topic)
	p, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req := kmsg.NewPtrCreateTopicsRequest()
	req.Topics = []kmsg.CreateTopicsRequestTopic{{Topic: topic, NumPartitions: 1, ReplicationFactor: 1}}
	if _, err = p.client.Request(ctx, req); err != nil {
		t.Fatal(err)
	}
	defer func() {
		r := kmsg.NewPtrDeleteTopicsRequest()
		r.TopicNames = []string{topic}
		bounded, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		p.client.Request(bounded, r)
	}()
	raw := []byte(`{"event_id":"event-1"}`)
	for i := 0; i < 2; i++ {
		if err = p.Publish(ctx, "event-1", raw); err != nil {
			t.Fatal(err)
		}
	}
	reader, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.ConsumePartitions(map[string]map[int32]kgo.Offset{topic: {0: kgo.NewOffset().AtStart()}}))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	count := 0
	for count < 2 {
		f := reader.PollFetches(ctx)
		if errs := f.Errors(); len(errs) > 0 {
			t.Fatal(errs)
		}
		f.EachRecord(func(r *kgo.Record) {
			if string(r.Key) != "event-1" || string(r.Value) != string(raw) {
				t.Error("identity/payload changed")
			}
			count++
		})
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if p.Publish(cancelled, "cancelled", raw) == nil {
		t.Fatal("cancelled publish acknowledged")
	}
	t.Setenv("VMCP_AUDIT_EXPORT_ALLOW_PLAINTEXT", "")
	secure, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	defer secure.Close()
	short, done := context.WithTimeout(ctx, time.Second)
	defer done()
	if secure.Publish(short, "tls-required", raw) == nil {
		t.Fatal("TLS downgraded")
	}
}
