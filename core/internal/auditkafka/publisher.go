// Package auditkafka is imported only by the Kafka audit exporter.
package auditkafka

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/bishoku/vmcp/core/internal/auditexport"
	"github.com/bishoku/vmcp/core/internal/netpolicy"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

type Publisher struct {
	client *kgo.Client
	topic  string
}

func FromEnv() (*Publisher, error) {
	brokers := strings.Split(os.Getenv("VMCP_AUDIT_KAFKA_BROKERS"), ",")
	for i, b := range brokers {
		brokers[i] = strings.TrimSpace(b)
		host, port, err := net.SplitHostPort(brokers[i])
		if err != nil || host == "" || port == "" {
			return nil, errors.New("Kafka brokers must be host:port addresses")
		}
	}
	topic := os.Getenv("VMCP_AUDIT_KAFKA_TOPIC")
	if !regexp.MustCompile(`^[a-zA-Z0-9._-]{1,249}$`).MatchString(topic) || topic == "." || topic == ".." {
		return nil, errors.New("invalid Kafka topic")
	}
	policy, err := netpolicy.FromEnv()
	if err != nil {
		return nil, errors.New("invalid audit egress policy")
	}
	secure := os.Getenv("VMCP_AUDIT_EXPORT_ALLOW_PLAINTEXT") != "true"
	tlsConfig, err := auditexport.TLSConfig()
	if err != nil {
		return nil, err
	}
	opts := []kgo.Opt{kgo.SeedBrokers(brokers...), kgo.RequiredAcks(kgo.AllISRAcks()), kgo.MaxBufferedRecords(1), kgo.MaxBufferedBytes(1 << 20), kgo.RecordDeliveryTimeout(5 * time.Second), kgo.ProducerBatchCompression(kgo.NoCompression()), kgo.Dialer(func(ctx context.Context, network, address string) (net.Conn, error) {
		bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		conn, err := policy.DialContext(bounded, network, address)
		if err != nil {
			return nil, err
		}
		if !secure {
			return conn, nil
		}
		cfg := tlsConfig.Clone()
		cfg.ServerName, _, err = net.SplitHostPort(address)
		if err != nil {
			conn.Close()
			return nil, err
		}
		secured := tls.Client(conn, cfg)
		if err = secured.HandshakeContext(bounded); err != nil {
			conn.Close()
			return nil, err
		}
		return secured, nil
	})}
	user, password, mechanism := os.Getenv("VMCP_AUDIT_KAFKA_USERNAME"), os.Getenv("VMCP_AUDIT_KAFKA_PASSWORD"), os.Getenv("VMCP_AUDIT_KAFKA_SASL")
	switch mechanism {
	case "":
		if user != "" || password != "" {
			return nil, errors.New("Kafka credentials require an explicit SASL mechanism")
		}
	case "PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512":
		if user == "" || password == "" {
			return nil, errors.New("Kafka SASL credentials required")
		}
		if mechanism == "PLAIN" {
			opts = append(opts, kgo.SASL(plain.Auth{User: user, Pass: password}.AsMechanism()))
		} else if mechanism == "SCRAM-SHA-256" {
			opts = append(opts, kgo.SASL(scram.Auth{User: user, Pass: password}.AsSha256Mechanism()))
		} else {
			opts = append(opts, kgo.SASL(scram.Auth{User: user, Pass: password}.AsSha512Mechanism()))
		}
	default:
		return nil, errors.New("unsupported Kafka SASL mechanism")
	}
	// Idempotent production is enabled by franz-go by default; never disable it.
	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, errors.New("invalid Kafka producer configuration")
	}
	return &Publisher{client, topic}, nil
}
func (p *Publisher) Publish(ctx context.Context, id string, raw []byte) error {
	record := &kgo.Record{Topic: p.topic, Key: []byte(id), Value: raw, Headers: []kgo.RecordHeader{{Key: "content-type", Value: []byte("application/json")}, {Key: "event-id", Value: []byte(id)}}}
	if err := p.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return errors.New("Kafka audit delivery not acknowledged")
	}
	return nil
}
func (p *Publisher) Close() { p.client.Close() }
