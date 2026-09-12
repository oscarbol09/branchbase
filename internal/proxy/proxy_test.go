package proxy

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/branchbase/branchbase/internal/config"
)

func TestServerLifecycle(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Proxy.ListenPort = 0 // Ephemeral port assigned by OS
	cfg.Connection.Host = "127.0.0.1"
	cfg.Connection.Port = 59999 // Backend doesn't need to accept for lifecycle check

	srv := NewServer(&cfg, t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Server.Start failed: %v", err)
	}

	if srv.listener == nil {
		t.Fatal("expected srv.listener to be non-nil after Start")
	}

	addr := srv.listener.Addr().String()

	// Dial proxy to trigger acceptLoop
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to dial proxy listener at %s: %v", addr, err)
	}
	_ = conn.Close()

	// Graceful shutdown
	done := make(chan struct{})
	go func() {
		_ = srv.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(3 * time.Second):
		t.Fatal("Server.Stop timed out")
	}
}

func TestServerStopUnstarted(t *testing.T) {
	cfg := config.DefaultConfig()
	srv := NewServer(&cfg, t.TempDir())

	// Stopping an unstarted server should not panic
	if err := srv.Stop(); err != nil {
		t.Fatalf("Stop on unstarted server returned error: %v", err)
	}
}

func TestServerStopIdempotent(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Proxy.ListenPort = 0
	cfg.Connection.Host = "127.0.0.1"
	cfg.Connection.Port = 59999

	srv := NewServer(&cfg, t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Server.Start failed: %v", err)
	}

	if err := srv.Stop(); err != nil {
		t.Fatalf("first Stop returned error: %v", err)
	}
	if err := srv.Stop(); err != nil {
		t.Fatalf("second Stop returned error: %v", err)
	}
}

func TestServerStopConcurrent(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Proxy.ListenPort = 0
	cfg.Connection.Host = "127.0.0.1"
	cfg.Connection.Port = 59999

	srv := NewServer(&cfg, t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Server.Start failed: %v", err)
	}

	const n = 8
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			errCh <- srv.Stop()
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent Stop returned error: %v", err)
		}
	}
}

// Connection refused on a closed local port is immediate (not a network timeout).
func TestHandleConnectionSendsErrorResponseOnDialFailure(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Proxy.ListenPort = 0
	cfg.Connection.Host = "127.0.0.1"
	cfg.Connection.Port = 59999

	srv := NewServer(&cfg, t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Server.Start failed: %v", err)
	}
	defer func() { _ = srv.Stop() }()

	addr := srv.listener.Addr().String()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to dial proxy listener at %s: %v", addr, err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))

	header := make([]byte, 5)
	if _, err := io.ReadFull(conn, header); err != nil {
		t.Fatalf("read ErrorResponse header: %v (want wire 'E' packet, not EOF)", err)
	}
	if header[0] != 'E' {
		t.Fatalf("got message type %q, want 'E'", header[0])
	}
	length := binary.BigEndian.Uint32(header[1:5])
	if length < 5 {
		t.Fatalf("implausible ErrorResponse length %d", length)
	}
	body := make([]byte, length-4)
	if _, err := io.ReadFull(conn, body); err != nil {
		t.Fatalf("read ErrorResponse body: %v", err)
	}

	fields := parseErrorFields(t, body[:len(body)-1])
	if fields['S'] != "FATAL" {
		t.Errorf("Severity (S) = %q, want FATAL", fields['S'])
	}
	if fields['C'] != "08001" {
		t.Errorf("Code (C) = %q, want 08001", fields['C'])
	}
	msg := fields['M']
	if !strings.Contains(msg, "BranchBase") {
		t.Errorf("Message missing BranchBase: %q", msg)
	}
	if !strings.Contains(msg, "127.0.0.1:59999") {
		t.Errorf("Message missing backend addr: %q", msg)
	}
	if !strings.Contains(msg, "connect") && !strings.Contains(strings.ToLower(msg), "refused") {
		t.Errorf("Message missing dial error: %q", msg)
	}
}

func parseErrorFields(t *testing.T, body []byte) map[byte]string {
	t.Helper()
	fields := make(map[byte]string)
	for len(body) > 0 {
		typ := body[0]
		body = body[1:]
		i := bytes.IndexByte(body, 0)
		if i < 0 {
			t.Fatal("unterminated error field")
		}
		fields[typ] = string(body[:i])
		body = body[i+1:]
	}
	return fields
}
