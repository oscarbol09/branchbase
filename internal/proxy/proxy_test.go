package proxy

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/branchbase/branchbase/internal/config"
	"github.com/branchbase/branchbase/internal/driver"
)

type mockDriver struct {
	mu          sync.Mutex
	branches    map[string]bool
	createCalls int
	createDelay time.Duration
	createErr   error
	existsErr   error
}

func newMockDriver() *mockDriver {
	return &mockDriver{
		branches: make(map[string]bool),
	}
}

func (m *mockDriver) Name() string { return "mock" }
func (m *mockDriver) Ping(ctx context.Context) error { return nil }
func (m *mockDriver) Close() error { return nil }

func (m *mockDriver) BranchExists(ctx context.Context, branchName string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.existsErr != nil {
		return false, m.existsErr
	}
	return m.branches[branchName], nil
}

func (m *mockDriver) CreateBranch(ctx context.Context, sourceBranch, targetBranch string) error {
	if m.createDelay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(m.createDelay):
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCalls++
	if m.createErr != nil {
		return m.createErr
	}
	m.branches[targetBranch] = true
	return nil
}

func (m *mockDriver) DeleteBranch(ctx context.Context, branchName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.branches, branchName)
	return nil
}

func (m *mockDriver) ListBranches(ctx context.Context) ([]driver.BranchInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []driver.BranchInfo
	for b := range m.branches {
		list = append(list, driver.BranchInfo{Name: b, Database: b})
	}
	return list, nil
}

func TestServerLifecycle(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Proxy.ListenPort = 0 // Ephemeral port assigned by OS
	cfg.Connection.Host = "127.0.0.1"
	cfg.Connection.Port = 59999 // Backend doesn't need to accept for lifecycle check

	srv := NewServer(&cfg, t.TempDir(), nil)
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
	srv := NewServer(&cfg, t.TempDir(), nil)

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

	srv := NewServer(&cfg, t.TempDir(), nil)
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

	srv := NewServer(&cfg, t.TempDir(), nil)
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
		srv := NewServer(&cfg, t.TempDir(), nil)
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

	srv := NewServer(&cfg, t.TempDir(), nil)
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

func TestProxyEnsureBranchExists_AlreadyExists(t *testing.T) {
	cfg := config.DefaultConfig()
	drv := newMockDriver()
	drv.branches["feature-existing"] = true

	srv := NewServer(&cfg, t.TempDir(), drv)
	if err := srv.ensureBranchExists("feature-existing", "main"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if drv.createCalls != 0 {
		t.Fatalf("expected 0 CreateBranch calls for existing branch, got %d", drv.createCalls)
	}
}

func TestProxyEnsureBranchExists_ProvisionsWhenMissing(t *testing.T) {
	cfg := config.DefaultConfig()
	drv := newMockDriver()

	srv := NewServer(&cfg, t.TempDir(), drv)
	if err := srv.ensureBranchExists("feature-new", "main"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if drv.createCalls != 1 {
		t.Fatalf("expected 1 CreateBranch call, got %d", drv.createCalls)
	}
	if !drv.branches["feature-new"] {
		t.Fatal("expected feature-new branch to be marked as existing")
	}
}

func TestProxyEnsureBranchExists_ConcurrentSameBranch(t *testing.T) {
	cfg := config.DefaultConfig()
	drv := newMockDriver()
	drv.createDelay = 15 * time.Millisecond // Simulate database clone latency

	srv := NewServer(&cfg, t.TempDir(), drv)

	const n = 20
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			errCh <- srv.ensureBranchExists("feature-race", "main")
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent ensureBranchExists returned error: %v", err)
		}
	}

	if drv.createCalls != 1 {
		t.Fatalf("expected exactly 1 CreateBranch call under race, got %d", drv.createCalls)
	}
	if srv.keyLock.keyCount() != 0 {
		t.Fatalf("expected keyLock to be clean (keyCount == 0), got %d", srv.keyLock.keyCount())
	}
}

func TestProxyEnsureBranchExists_ConcurrentDifferentBranches(t *testing.T) {
	cfg := config.DefaultConfig()
	drv := newMockDriver()
	drv.createDelay = 10 * time.Millisecond

	srv := NewServer(&cfg, t.TempDir(), drv)

	const n = 10
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	wg.Add(n)

	for i := 0; i < n; i++ {
		branch := fmt.Sprintf("feature-%d", i)
		go func(b string) {
			defer wg.Done()
			errCh <- srv.ensureBranchExists(b, "main")
		}(branch)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent ensureBranchExists returned error: %v", err)
		}
	}

	if drv.createCalls != n {
		t.Fatalf("expected %d CreateBranch calls for distinct branches, got %d", n, drv.createCalls)
	}
	if srv.keyLock.keyCount() != 0 {
		t.Fatalf("expected keyLock to be clean (keyCount == 0), got %d", srv.keyLock.keyCount())
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

func TestProxyHandleConnection_FailClosedOnRewriteError(t *testing.T) {
	// 1. Set up a mock backend listener
	backendListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create mock backend listener: %v", err)
	}
	defer backendListener.Close()

	backendPort := backendListener.Addr().(*net.TCPAddr).Port

	// Channel to capture whatever payload the backend receives
	receivedOnBackend := make(chan []byte, 1)
	go func() {
		conn, err := backendListener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 1024)
		n, _ := conn.Read(buf)
		if n > 0 {
			receivedOnBackend <- buf[:n]
		} else {
			receivedOnBackend <- []byte{}
		}
	}()

	// 2. Set up proxy with targetDB that exceeds 63 bytes (Postgres NAMEDATALEN limit)
	cfg := config.DefaultConfig()
	cfg.Proxy.ListenPort = 0
	cfg.Connection.Host = "127.0.0.1"
	cfg.Connection.Port = backendPort
	// BaseDatabase with 60 characters so that BaseDatabase + "_" + branch exceeds 63 bytes
	cfg.Connection.BaseDatabase = "a_very_long_base_database_name_that_leaves_no_room_for_branches"

	// Create a temp repository with active branch "feature_xyz"
	tempDir := t.TempDir()
	gitDir := filepath.Join(tempDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("failed to create fake .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature_xyz\n"), 0644); err != nil {
		t.Fatalf("failed to write fake HEAD: %v", err)
	}

	srv := NewServer(&cfg, tempDir, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Server.Start failed: %v", err)
	}
	defer func() { _ = srv.Stop() }()

	addr := listenerAddr(srv)

	// 3. Connect client to proxy and send valid startup packet requesting "myapp_dev"
	clientConn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to dial proxy: %v", err)
	}
	defer clientConn.Close()

	var buf bytes.Buffer
	buf.Write([]byte{0, 0, 0, 0}) // placeholder
	var proto [4]byte
	binary.BigEndian.PutUint32(proto[:], 196608) // ProtocolVersion3
	buf.Write(proto[:])
	buf.WriteString("user")
	buf.WriteByte(0)
	buf.WriteString("postgres")
	buf.WriteByte(0)
	buf.WriteString("database")
	buf.WriteByte(0)
	buf.WriteString("myapp_dev")
	buf.WriteByte(0)
	buf.WriteByte(0)
	data := buf.Bytes()
	binary.BigEndian.PutUint32(data[0:4], uint32(len(data)))

	if _, err := clientConn.Write(data); err != nil {
		t.Fatalf("failed to send startup packet: %v", err)
	}

	// 4. Verify backend receives NO payload. The proxy MUST abort and fail-closed!
	select {
	case payload := <-receivedOnBackend:
		if len(payload) > 0 {
			t.Fatalf("SECURITY VIOLATION: proxy forwarded %d bytes to backend despite RewriteDatabase error! Raw packet was forwarded.", len(payload))
		}
	case <-time.After(500 * time.Millisecond):
		// Mock backend read timed out with 0 bytes, which is also valid fail-closed behavior
	}
}

