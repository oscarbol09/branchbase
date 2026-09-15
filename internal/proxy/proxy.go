package proxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/branchbase/branchbase/internal/config"
	"github.com/branchbase/branchbase/internal/driver"
	"github.com/branchbase/branchbase/internal/git"
	"github.com/branchbase/branchbase/internal/proxy/pgwire"
)

// Server represents the transparent TCP proxy
type Server struct {
	cfg      *config.Config
	repoPath string
	drv      driver.Driver
	keyLock  *keyedMutex
	mu       sync.Mutex
	listener net.Listener
	quit     chan struct{}
	wg       sync.WaitGroup
	stopOnce sync.Once
}

// NewServer initializes a new transparent proxy server
func NewServer(cfg *config.Config, repoPath string, drv driver.Driver) *Server {
	return &Server{
		cfg:      cfg,
		repoPath: repoPath,
		drv:      drv,
		keyLock:  newKeyedMutex(),
		quit:     make(chan struct{}),
	}
}

// Start begins listening for incoming application database connections
func (s *Server) Start(ctx context.Context) error {
	addr := net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", s.cfg.Proxy.ListenPort))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to bind proxy to %s: %w", addr, err)
	}

	s.mu.Lock()
	select {
	case <-s.quit:
		s.mu.Unlock()
		_ = listener.Close()
		return fmt.Errorf("proxy already stopped")
	default:
	}
	s.listener = listener
	s.wg.Add(1)
	s.mu.Unlock()

	log.Printf("[BranchBase Proxy] 🚀 Listening on %s -> Forwarding to backend %s:%d",
		addr, s.cfg.Connection.Host, s.cfg.Connection.Port)

	go s.acceptLoop()

	return nil
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()

	s.mu.Lock()
	ln := s.listener
	s.mu.Unlock()
	if ln == nil {
		return
	}

	for {
		clientConn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
				log.Printf("[BranchBase Proxy] Accept error: %v", err)
				continue
			}
		}

		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			s.handleConnection(c)
		}(clientConn)
	}
}

// ensureBranchExists checks whether targetBranch database exists, and if not,
// provisions it from defaultBranch using double-checked locking per branch.
func (s *Server) ensureBranchExists(targetBranch, defaultBranch string) error {
	if s.drv == nil {
		return nil
	}

	fastCtx, fastCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer fastCancel()

	// Fast path: check existence without lock
	exists, err := s.drv.BranchExists(fastCtx, targetBranch)
	if err != nil {
		return fmt.Errorf("failed to check branch existence for %q: %w", targetBranch, err)
	}
	if exists {
		return nil
	}

	// Slow path: acquire keyed lock to prevent duplicate provisioning races
	if s.keyLock != nil {
		unlock := s.keyLock.Lock(targetBranch)
		defer unlock()

		// Dedicated timeout once lock is acquired to prevent starvation under contention
		provCtx, provCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer provCancel()

		// Re-check existence under lock
		exists, err = s.drv.BranchExists(provCtx, targetBranch)
		if err != nil {
			return fmt.Errorf("failed to check branch existence under lock for %q: %w", targetBranch, err)
		}
		if exists {
			return nil
		}

		log.Printf("[BranchBase Proxy] 🪄 JIT provisioning database for branch %q from %q...", targetBranch, defaultBranch)
		if err := s.drv.CreateBranch(provCtx, defaultBranch, targetBranch); err != nil {
			return fmt.Errorf("failed to JIT provision branch %q: %w", targetBranch, err)
		}
		log.Printf("[BranchBase Proxy] ✅ JIT provisioned database for branch %q", targetBranch)
		return nil
	}

	// Fallback if keyLock is nil (e.g. in tests)
	provCtx, provCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer provCancel()

	log.Printf("[BranchBase Proxy] 🪄 JIT provisioning database for branch %q from %q...", targetBranch, defaultBranch)
	if err := s.drv.CreateBranch(provCtx, defaultBranch, targetBranch); err != nil {
		return fmt.Errorf("failed to JIT provision branch %q: %w", targetBranch, err)
	}
	log.Printf("[BranchBase Proxy] ✅ JIT provisioned database for branch %q", targetBranch)
	return nil
}

func (s *Server) handleConnection(clientConn net.Conn) {
	defer func() {
		_ = clientConn.Close()
	}()

	// 1. Resolve active Git branch for this repository
	activeBranch, err := git.ResolveCurrentBranch(s.repoPath)
	if err != nil {
		activeBranch = s.cfg.Proxy.DefaultBranch
	}
	sanitizedBranch := git.SanitizeBranchName(activeBranch)
	targetDB := s.cfg.DatabaseNameForBranch(sanitizedBranch)

	defaultBranch := s.cfg.Proxy.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	// 1b. JIT branch provisioning if branch differs from default
	if s.drv != nil && sanitizedBranch != git.SanitizeBranchName(defaultBranch) {
		if err := s.ensureBranchExists(sanitizedBranch, defaultBranch); err != nil {
			log.Printf("[BranchBase Proxy] ❌ JIT branch provisioning failed for %q: %v", sanitizedBranch, err)
			return
		}
	}

	log.Printf("[BranchBase Proxy] Routing client connection -> Branch: %q (DB: %q)", activeBranch, targetDB)

	// 2. Connect to backend database server
	backendAddr := net.JoinHostPort(s.cfg.Connection.Host, fmt.Sprintf("%d", s.cfg.Connection.Port))
	backendConn, err := net.DialTimeout("tcp", backendAddr, 5*time.Second)
	if err != nil {
		log.Printf("[BranchBase Proxy] ❌ Failed to connect to backend %s: %v", backendAddr, err)
		return
	}
	defer func() {
		_ = backendConn.Close()
	}()

	// 3. PostgreSQL Wire Protocol Handshake Inspection & Rewriting
	packet, err := pgwire.ReadStartupPacket(clientConn)
	if err != nil {
		log.Printf("[BranchBase Proxy] Error reading client startup packet: %v", err)
		return
	}

	// Handle SSL negotiation: reply 'N' (SSL unsupported) so client continues in plaintext
	if pgwire.IsSSLRequest(packet) {
		if _, err := clientConn.Write([]byte{'N'}); err != nil {
			log.Printf("[BranchBase Proxy] Error responding to SSL request: %v", err)
			return
		}
		packet, err = pgwire.ReadStartupPacket(clientConn)
		if err != nil {
			log.Printf("[BranchBase Proxy] Error reading startup packet after SSL negotiation: %v", err)
			return
		}
	}

	// Rewrite target database to the branch-specific database.
	// Invariant: fail-closed. If rewriting fails (e.g., invalid database identifier,
	// length exceeding 63 bytes, or malformed startup packet), abort immediately.
	// NEVER forward the raw un-rewritten packet, as that would route queries to the
	// client's default/base database (silent data corruption risk).
	rewrittenPacket, err := pgwire.RewriteDatabase(packet, targetDB)
	if err != nil {
		log.Printf("[BranchBase Proxy] ❌ Failed to rewrite database to %q: %v. Aborting connection.", targetDB, err)
		return
	}

	if _, err := backendConn.Write(rewrittenPacket); err != nil {
		log.Printf("[BranchBase Proxy] Error forwarding startup packet to backend: %v", err)
		return
	}

	// 4. Bidirectional streaming. When either direction finishes, close both
	// sockets so the other io.Copy unblocks, then drain both goroutines before
	// returning (avoids leaking a blocked copy + FD under half-close).
	errChan := make(chan error, 2)
	go func() {
		_, err := io.Copy(backendConn, clientConn)
		errChan <- err
	}()
	go func() {
		_, err := io.Copy(clientConn, backendConn)
		errChan <- err
	}()

	<-errChan
	_ = clientConn.Close()
	_ = backendConn.Close()
	<-errChan
}

// Stop gracefully shuts down the proxy server.
// It is safe to call Stop more than once, including concurrently.
func (s *Server) Stop() error {
	s.stopOnce.Do(func() {
		s.mu.Lock()
		close(s.quit)
		ln := s.listener
		s.listener = nil
		s.mu.Unlock()
		if ln != nil {
			_ = ln.Close()
		}
	})
	s.wg.Wait()
	log.Println("[BranchBase Proxy] Stopped.")
	return nil
}
