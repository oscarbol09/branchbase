package proxy

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/branchbase/branchbase/internal/config"
	"github.com/branchbase/branchbase/internal/driver"
	"github.com/branchbase/branchbase/internal/git"
	"github.com/branchbase/branchbase/internal/proxy/pgwire"
)

// Server is the transparent TCP & UNIX socket proxy forwarder
type Server struct {
	cfg          *config.Config
	repoPath     string
	drv          driver.Driver
	listener     net.Listener
	wg           sync.WaitGroup
	keyLock      *keyedMutex
	mu           sync.Mutex
	stopOnce     sync.Once
	activeConns  map[net.Conn]struct{}
	activeMu     sync.Mutex
	tlsConfig    *tls.Config
	drainTimeout time.Duration
}

// NewServer initializes a new transparent proxy server
func NewServer(cfg *config.Config, repoPath string, drv driver.Driver) *Server {
	return &Server{
		cfg:          cfg,
		repoPath:     repoPath,
		drv:          drv,
		keyLock:      newKeyedMutex(),
		activeConns:  make(map[net.Conn]struct{}),
		drainTimeout: 3 * time.Second,
	}
}

// Start begins listening on the configured TCP port or UNIX socket and routing traffic
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Initialize TLS if enabled
	if s.cfg.Proxy.TLS.Enabled {
		tlsCfg, err := s.setupTLSConfig()
		if err != nil {
			return fmt.Errorf("failed to setup TLS configuration: %w", err)
		}
		s.tlsConfig = tlsCfg
	}

	var l net.Listener
	var err error

	socketPath := s.cfg.Proxy.SocketPath
	if socketPath != "" {
		_ = os.Remove(socketPath)
		l, err = net.Listen("unix", socketPath)
		if err != nil {
			return fmt.Errorf("failed to bind proxy to UNIX socket %s: %w", socketPath, err)
		}
		log.Printf("[BranchBase Proxy] 🌿 Transparent proxy listening on UNIX socket: %s", socketPath)
	} else {
		addr := fmt.Sprintf(":%d", s.cfg.Proxy.ListenPort)
		l, err = net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("failed to bind proxy to %s: %w", addr, err)
		}
		log.Printf("[BranchBase Proxy] 🌿 Transparent proxy listening on TCP %s", l.Addr().String())
	}

	s.listener = l

	s.wg.Add(1)
	go s.acceptLoop(ctx)

	return nil
}

func (s *Server) setupTLSConfig() (*tls.Config, error) {
	if s.cfg.Proxy.TLS.CertFile != "" && s.cfg.Proxy.TLS.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(s.cfg.Proxy.TLS.CertFile, s.cfg.Proxy.TLS.KeyFile)
		if err != nil {
			return nil, err
		}
		return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
	}

	// Auto-generate self-signed certificate for dev
	cert, err := generateSelfSignedCert()
	if err != nil {
		return nil, fmt.Errorf("failed to generate self-signed dev certificate: %w", err)
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
}

func generateSelfSignedCert() (tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"BranchBase Local Development"},
			CommonName:   "localhost",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost", "127.0.0.1"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	return tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
	}, nil
}

// Stop gracefully stops the proxy, draining active connections before terminating
func (s *Server) Stop() error {
	var stopErr error
	s.stopOnce.Do(func() {
		s.mu.Lock()
		l := s.listener
		s.listener = nil
		s.mu.Unlock()

		if l != nil {
			stopErr = l.Close()
		}

		if s.cfg.Proxy.SocketPath != "" {
			_ = os.Remove(s.cfg.Proxy.SocketPath)
		}

		// Graceful drain with timeout
		drainDone := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(drainDone)
		}()

		select {
		case <-drainDone:
			// Drained gracefully
		case <-time.After(s.drainTimeout):
			// Timeout expired, forcefully close remaining active connections
			s.closeActiveConns()
			<-drainDone
		}
	})
	return stopErr
}

func (s *Server) addActiveConn(c net.Conn) {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	s.activeConns[c] = struct{}{}
}

func (s *Server) removeActiveConn(c net.Conn) {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	delete(s.activeConns, c)
}

func (s *Server) closeActiveConns() {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	for c := range s.activeConns {
		_ = c.Close()
	}
}

func (s *Server) acceptLoop(ctx context.Context) {
	defer s.wg.Done()

	for {
		s.mu.Lock()
		l := s.listener
		s.mu.Unlock()

		if l == nil {
			return
		}

		clientConn, err := l.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				s.mu.Lock()
				isClosing := (s.listener == nil)
				s.mu.Unlock()
				if isClosing {
					return
				}
				log.Printf("[BranchBase Proxy] Accept error: %v", err)
				time.Sleep(50 * time.Millisecond)
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

func (s *Server) ensureBranchExists(targetBranch, defaultBranch string) error {
	if s.drv == nil {
		return nil
	}

	fastCtx, fastCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer fastCancel()

	exists, err := s.drv.BranchExists(fastCtx, targetBranch)
	if err != nil {
		return fmt.Errorf("failed to check branch existence for %q: %w", targetBranch, err)
	}
	if exists {
		return nil
	}

	if s.keyLock != nil {
		unlock := s.keyLock.Lock(targetBranch)
		defer unlock()

		provCtx, provCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer provCancel()

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

	provCtx, provCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer provCancel()

	log.Printf("[BranchBase Proxy] 🪄 JIT provisioning database for branch %q from %q...", targetBranch, defaultBranch)
	if err := s.drv.CreateBranch(provCtx, defaultBranch, targetBranch); err != nil {
		return fmt.Errorf("failed to JIT provision branch %q: %w", targetBranch, err)
	}
	log.Printf("[BranchBase Proxy] ✅ JIT provisioned database for branch %q", targetBranch)
	return nil
}

func (s *Server) dialBackend() (net.Conn, string, error) {
	socketPath := s.cfg.Connection.SocketPath
	if socketPath != "" {
		conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
		return conn, socketPath, err
	}

	host := s.cfg.Connection.Host
	if strings.HasPrefix(host, "/") {
		conn, err := net.DialTimeout("unix", host, 5*time.Second)
		return conn, host, err
	}

	backendAddr := net.JoinHostPort(host, fmt.Sprintf("%d", s.cfg.Connection.Port))
	conn, err := net.DialTimeout("tcp", backendAddr, 5*time.Second)
	return conn, backendAddr, err
}

func (s *Server) handleConnection(clientConn net.Conn) {
	s.addActiveConn(clientConn)
	defer func() {
		s.removeActiveConn(clientConn)
		_ = clientConn.Close()
	}()

	// 1. Resolve active Git branch
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

	// 1b. JIT branch provisioning
	if s.drv != nil && sanitizedBranch != git.SanitizeBranchName(defaultBranch) {
		if err := s.ensureBranchExists(sanitizedBranch, defaultBranch); err != nil {
			log.Printf("[BranchBase Proxy] ❌ JIT branch provisioning failed for %q: %v", sanitizedBranch, err)
			errMsg := fmt.Sprintf("BranchBase: failed to provision database for branch %q: %v", sanitizedBranch, err)
			_, _ = clientConn.Write(pgwire.BuildErrorResponse("FATAL", "3D000", errMsg))
			return
		}
	}

	log.Printf("[BranchBase Proxy] Routing client connection -> Branch: %q (DB: %q)", activeBranch, targetDB)

	// 2. Connect to backend database server
	backendConn, backendAddr, err := s.dialBackend()
	if err != nil {
		log.Printf("[BranchBase Proxy] ❌ Failed to connect to backend %s: %v", backendAddr, err)
		errMsg := fmt.Sprintf("BranchBase: failed to connect to database backend %s: %v", backendAddr, err)
		_, _ = clientConn.Write(pgwire.BuildErrorResponse("FATAL", "08006", errMsg))
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

	// Handle CancelRequest (16 bytes, code 80877102)
	if pgwire.IsCancelRequest(packet) {
		log.Printf("[BranchBase Proxy] 🛑 Forwarding CancelRequest packet to backend...")
		if _, err := backendConn.Write(packet); err != nil {
			log.Printf("[BranchBase Proxy] Error forwarding CancelRequest to backend: %v", err)
		}
		return
	}

	// Handle SSL negotiation
	if pgwire.IsSSLRequest(packet) {
		if s.cfg.Proxy.TLS.Enabled && s.tlsConfig != nil {
			if _, err := clientConn.Write([]byte{'S'}); err != nil {
				log.Printf("[BranchBase Proxy] Error responding 'S' to SSL request: %v", err)
				return
			}
			tlsConn := tls.Server(clientConn, s.tlsConfig)
			if err := tlsConn.Handshake(); err != nil {
				log.Printf("[BranchBase Proxy] TLS server handshake error: %v", err)
				return
			}
			clientConn = tlsConn
			packet, err = pgwire.ReadStartupPacket(clientConn)
			if err != nil {
				log.Printf("[BranchBase Proxy] Error reading decrypted startup packet: %v", err)
				return
			}
		} else {
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
	}

	// Rewrite database identifier
	rewrittenPacket, err := pgwire.RewriteDatabase(packet, targetDB)
	if err != nil {
		log.Printf("[BranchBase Proxy] ❌ Failed to rewrite database to %q: %v. Aborting connection.", targetDB, err)
		errMsg := fmt.Sprintf("BranchBase: failed to route database to %q: %v", targetDB, err)
		_, _ = clientConn.Write(pgwire.BuildErrorResponse("FATAL", "3D000", errMsg))
		return
	}

	if _, err := backendConn.Write(rewrittenPacket); err != nil {
		log.Printf("[BranchBase Proxy] Error forwarding startup packet to backend: %v", err)
		return
	}

	// 4. Bidirectional streaming
	var transferWg sync.WaitGroup
	transferWg.Add(2)

	go func() {
		defer transferWg.Done()
		_, _ = io.Copy(backendConn, clientConn)
		if tcpBack, ok := backendConn.(*net.TCPConn); ok {
			_ = tcpBack.CloseWrite()
		} else {
			_ = backendConn.Close()
		}
	}()

	go func() {
		defer transferWg.Done()
		_, _ = io.Copy(clientConn, backendConn)
		if tcpCli, ok := clientConn.(*net.TCPConn); ok {
			_ = tcpCli.CloseWrite()
		} else {
			_ = clientConn.Close()
		}
	}()

	transferWg.Wait()
}
