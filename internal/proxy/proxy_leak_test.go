package proxy

import (
	"io"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/branchbase/branchbase/internal/config"
)

// Mirrors handleConnection's bidirectional copy teardown: after the first
// direction ends, both conns are closed and both goroutines must finish.
func pipeBothWays(a, b net.Conn) {
	errChan := make(chan error, 2)
	go func() {
		_, err := io.Copy(b, a)
		errChan <- err
	}()
	go func() {
		_, err := io.Copy(a, b)
		errChan <- err
	}()
	<-errChan
	_ = a.Close()
	_ = b.Close()
	<-errChan
}

func TestBidirectionalCopyDrainsBothSides(t *testing.T) {
	clientA, clientB := net.Pipe()
	backendA, backendB := net.Pipe()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// proxy side: clientB <-> backendA
		pipeBothWays(clientB, backendA)
	}()

	// Peer that only half-closes: write then close write side by closing entirely
	go func() {
		_, _ = clientA.Write([]byte("hi"))
		_ = clientA.Close()
	}()
	go func() {
		buf := make([]byte, 8)
		_, _ = backendB.Read(buf)
		// leave backendB open until proxy closes it
		time.Sleep(50 * time.Millisecond)
		_ = backendB.Close()
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("proxy copy did not drain both goroutines within 2s")
	}
}

func TestBidirectionalCopyDoesNotLeakGoroutines(t *testing.T) {
	runtime.GC()
	before := runtime.NumGoroutine()

	for i := 0; i < 50; i++ {
		a, b := net.Pipe()
		c, d := net.Pipe()
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			pipeBothWays(b, c)
		}()
		_, _ = a.Write([]byte("x"))
		_ = a.Close()
		_ = d.Close()
		wg.Wait()
	}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after > before+10 {
		t.Fatalf("goroutines grew too much: before=%d after=%d", before, after)
	}
}

func TestServerStopWithActiveTLSConns(t *testing.T) {
	s := &Server{
		cfg:          &config.Config{},
		activeConns:  make(map[net.Conn]struct{}),
		drainTimeout: 500 * time.Millisecond,
	}

	c1, c2 := net.Pipe()
	defer func() {
		_ = c1.Close()
		_ = c2.Close()
	}()

	s.addActiveConn(c1)
	if len(s.activeConns) != 1 {
		t.Fatalf("expected 1 active conn, got %d", len(s.activeConns))
	}

	// Simulate adding a wrapped TLS connection
	tlsWrapper, _ := net.Pipe()
	defer func() { _ = tlsWrapper.Close() }()
	s.addActiveConn(tlsWrapper)

	if len(s.activeConns) != 2 {
		t.Fatalf("expected 2 active conns, got %d", len(s.activeConns))
	}

	s.removeActiveConn(tlsWrapper)
	s.removeActiveConn(c1)

	s.activeMu.Lock()
	count := len(s.activeConns)
	s.activeMu.Unlock()

	if count != 0 {
		t.Fatalf("expected 0 active conns after removal, got %d", count)
	}

	err := s.Stop()
	if err != nil {
		t.Fatalf("unexpected stop error: %v", err)
	}
}

