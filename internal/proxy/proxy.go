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
)

// Server represents the transparent TCP proxy
type Server struct {
	cfg      *config.Config
	repoPath string
	listener net.Listener
	quit     chan struct{}
	wg       sync.WaitGroup
}

// NewServer initializes a new transparent proxy server
func NewServer(cfg *config.Config, repoPath string) *Server {
	return &Server{
		cfg:      cfg,
		repoPath: repoPath,
		quit:     make(chan struct{}),
	}
}

// Start begins listening for incoming application database connections
func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf("127.0.0.1:%d", s.cfg.Proxy.ListenPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to bind proxy to %s: %w", addr, err)
	}
	s.listener = listener

	log.Printf("[BranchBase Proxy] 🚀 Listening on %s -> Forwarding to backend %s:%d",
		addr, s.cfg.Connection.Host, s.cfg.Connection.Port)

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		clientConn, err := s.listener.Accept()
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

func (s *Server) handleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	// 1. Resolve active Git branch for this repository
	activeBranch, err := git.ResolveCurrentBranch(s.repoPath)
	if err != nil {
		activeBranch = s.cfg.Proxy.DefaultBranch
	}
	sanitizedBranch := git.SanitizeBranchName(activeBranch)
	targetDB := s.cfg.DatabaseNameForBranch(sanitizedBranch)

	log.Printf("[BranchBase Proxy] Routing client connection -> Branch: %q (DB: %q)", activeBranch, targetDB)

	// 2. Connect to backend database server
	backendAddr := fmt.Sprintf("%s:%d", s.cfg.Connection.Host, s.cfg.Connection.Port)
	backendConn, err := net.DialTimeout("tcp", backendAddr, 5*time.Second)
	if err != nil {
		log.Printf("[BranchBase Proxy] ❌ Failed to connect to backend %s: %v", backendAddr, err)
		return
	}
	defer backendConn.Close()

	// 3. Bidirectional streaming (zero-overhead pipe)
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
}

// Stop gracefully shuts down the proxy server
func (s *Server) Stop() error {
	close(s.quit)
	if s.listener != nil {
		_ = s.listener.Close()
	}
	s.wg.Wait()
	log.Println("[BranchBase Proxy] Stopped.")
	return nil
}
