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
	"github.com/branchbase/branchbase/internal/git"
	"github.com/branchbase/branchbase/internal/proxy/pgwire"
)

// Server represents the transparent TCP proxy.
type Server struct {
	cfg        *config.Config
	repoPath   string
	listener   net.Listener
	listenerMu sync.Mutex
	quit       chan struct{}
	wg         sync.WaitGroup
	stopOnce   sync.Once
}

// NewServer initializes a new transparent proxy server.
func NewServer(cfg *config.Config, repoPath string) *Server {
	return &Server{
		cfg:      cfg,
		repoPath: repoPath,
		quit:     make(chan struct{}),
	}
}

// Start begins listening for incoming application database connections.
func (s *Server) Start(ctx context.Context) error {
	addr := net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", s.cfg.Proxy.ListenPort))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to bind proxy to %s: %w", addr, err)
	}

	s.listenerMu.Lock()
	select {
	case <-s.quit:
		s.listenerMu.Unlock()
		_ = listener.Close()
		return fmt.Errorf("proxy server already stopped")
	default:
		s.listener = listener
		s.wg.Add(1)
		go s.acceptLoop(listener)
	}
	s.listenerMu.Unlock()

	log.Printf("[BranchBase Proxy] 🚀 Listening on %s -> Forwarding to backend %s:%d",
		addr, s.cfg.Connection.Host, s.cfg.Connection.Port)

	return nil
}

// Listener returns the active net.Listener, or nil if the server is not listening.
func (s *Server) Listener() net.Listener {
	s.listenerMu.Lock()
	defer s.listenerMu.Unlock()
	return s.listener
}

func (s *Server) acceptLoop(listener net.Listener) {
	defer s.wg.Done()

	for {
		clientConn, err := listener.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
				log.Printf("[BranchBase Proxy] Accept error: %v", err)
				continue
			}
		}

		s.listenerMu.Lock()
		select {
		case <-s.quit:
			s.listenerMu.Unlock()
			_ = clientConn.Close()
			return
		default:
			s.wg.Add(1)
			go func(c net.Conn) {
				defer s.wg.Done()
				s.handleConnection(c)
			}(clientConn)
		}
		s.listenerMu.Unlock()
	}
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

	// Rewrite target database to the branch-specific database
	rewrittenPacket, err := pgwire.RewriteDatabase(packet, targetDB)
	if err != nil {
		log.Printf("[BranchBase Proxy] Warning: could not rewrite database, forwarding raw: %v", err)
		rewrittenPacket = packet
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
		s.listenerMu.Lock()
		close(s.quit)
		l := s.listener
		s.listener = nil
		s.listenerMu.Unlock()

		if l != nil {
			_ = l.Close()
		}
	})
	s.wg.Wait()
	log.Println("[BranchBase Proxy] Stopped.")
	return nil
}
