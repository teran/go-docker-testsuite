//go:build integration

package kafka

import (
    "context"
    "net"
    "strings"
    "testing"
    "time"

    "github.com/teran/go-docker-testsuite/applications/kafka"
)

func TestKafkaVersion431(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
    defer cancel()

    k, err := kafka.NewWithImage(ctx, "index.docker.io/apache/kafka:4.3.1")
    if err != nil {
        t.Fatalf("failed to create kafka container: %v", err)
    }
    defer func() { _ = k.Close(context.Background()) }()

    url, err := k.GetBrokerURL(ctx)
    if err != nil {
        t.Fatalf("GetBrokerURL error: %v", err)
    }
    addr := strings.TrimPrefix(url, "tcp://")
    conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
    if err != nil {
        t.Fatalf("unable to connect to broker at %s: %v", addr, err)
    }
    _ = conn.Close()
}
