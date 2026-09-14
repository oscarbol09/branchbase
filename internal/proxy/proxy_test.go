package proxy

import (
	"context"
	"net"
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

	addr := listenerAddr(srv)
	if addr == "" {
		t.Fatal("expected srv.listener to be non-nil after Start")
	}

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

func TestServerStartStopRace(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Proxy.ListenPort = 0
	cfg.Connection.Host = "127.0.0.1"
	cfg.Connection.Port = 59999

	const n = 50
	for i := 0; i < n; i++ {
		srv := NewServer(&cfg, t.TempDir())
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = srv.Start(ctx)
		}()
		go func() {
			defer wg.Done()
			_ = srv.Stop()
		}()
		wg.Wait()
		cancel()
		if err := srv.Stop(); err != nil {
			t.Fatalf("iteration %d: final Stop: %v", i, err)
		}
	}
}

func TestServerAcceptLoopStopRace(t *testing.T) {
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

	addr := listenerAddr(srv)
	if addr == "" {
		t.Fatal("expected listen addr after Start")
	}

	const n = 8
	var wg sync.WaitGroup
	wg.Add(n + 1)
	go func() {
		defer wg.Done()
		_ = srv.Stop()
	}()
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
			if err == nil {
				_ = conn.Close()
			}
			_ = listenerAddr(srv)
		}()
	}
	wg.Wait()

	if err := srv.Stop(); err != nil {
		t.Fatalf("final Stop: %v", err)
	}
}

func listenerAddr(s *Server) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}
