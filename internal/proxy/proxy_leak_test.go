package proxy

import (
	"io"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"
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
