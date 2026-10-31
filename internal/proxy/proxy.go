package proxy

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
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
	"github.com/branchbase/branchbase/internal/proxy/mysqlwire"
	"github.com/branchbase/branchbase/internal/proxy/pgwire"
)

// Server is the transparent TCP & UNIX socket proxy forwarder
type Server struct {
	cfg             *config.Config
	repoPath        string
	drv             driver.Driver
	listener        net.Listener
	wg              sync.WaitGroup
	keyLock         *keyedMutex
	mu              sync.Mutex
	stopped         bool
	activeConns     map[net.Conn]struct{}
	connectionSlots chan struct{}
	activeMu        sync.Mutex
	branchCheckMu   sync.Mutex
	branchChecks    map[string]struct{}
	provisionedMu   sync.RWMutex
	provisioned     map[string]struct{}
	tlsConfig       *tls.Config
	drainTimeout    time.Duration
}

const (
	maxConcurrentConnections = 256
	startupTimeout           = 10 * time.Second
)

// NewServer initializes a new transparent proxy server
func NewServer(cfg *config.Config, repoPath string, drv driver.Driver) *Server {
	return &Server{
		cfg:             cfg,
		repoPath:        repoPath,
		drv:             drv,
		keyLock:         newKeyedMutex(),
		activeConns:     make(map[net.Conn]struct{}),
		connectionSlots: make(chan struct{}, maxConcurrentConnections),
		branchChecks:    make(map[string]struct{}),
		provisioned:     make(map[string]struct{}),
		drainTimeout:    500 * time.Millisecond,
	}
}

// SetDrainTimeout configures the maximum duration to wait for active connections to drain
func (s *Server) SetDrainTimeout(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drainTimeout = d
}

// Start begins listening on the configured TCP port or UNIX socket and routing traffic
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return fmt.Errorf("server is already stopped")
	}
	if s.listener != nil {
		return fmt.Errorf("server is already running")
	}

	if s.cfg != nil {
		if s.cfg.Driver == "sqlite" || s.cfg.Driver == "mongodb" {
			return fmt.Errorf("transparent wire proxy does not support %q driver (use 'branchbase switch' or environment variables instead)", s.cfg.Driver)
		}
		if s.cfg.Driver != "" && s.cfg.Driver != "postgres" && s.cfg.Driver != "mysql" && s.cfg.Driver != "mariadb" {
			return fmt.Errorf("unsupported proxy driver %q: only postgres, mysql, and mariadb wire protocols are supported", s.cfg.Driver)
		}
	}

	// Initialize TLS if enabled
	if s.cfg.Proxy.TLS.Enabled {
		tlsCfg, err := s.setupTLSConfig()
		if err != nil {
			return fmt.Errorf("failed to setup TLS configuration: %w", err)
		}
		s.tlsConfig = tlsCfg
		log.Printf("[BranchBase Proxy] ⚠️ Warning: TLS termination is enabled on the client side, but backend connections to %s are currently unencrypted (plaintext wire protocol).", s.cfg.Connection.Host)
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
		host := strings.TrimSpace(s.cfg.Proxy.ListenHost)
		if host == "" {
			host = "127.0.0.1"
		}
		addr := net.JoinHostPort(host, fmt.Sprintf("%d", s.cfg.Proxy.ListenPort))
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
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return nil
	}
	s.stopped = true
	l := s.listener
	s.listener = nil
	s.mu.Unlock()

	var stopErr error
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
		if err := c.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close c: %v\n", err)
		}
	}
}

func (s *Server) validateBranchIdentity(branch string) error {
	s.branchCheckMu.Lock()
	if _, checked := s.branchChecks[branch]; checked {
		s.branchCheckMu.Unlock()
		return nil
	}
	s.branchCheckMu.Unlock()

	if err := git.ValidateBranchNameUnique(s.repoPath, branch); err != nil {
		return err
	}
	s.branchCheckMu.Lock()
	s.branchChecks[branch] = struct{}{}
	s.branchCheckMu.Unlock()
	return nil
}

func (s *Server) acceptLoop(ctx context.Context) {
	defer s.wg.Done()

	stopWait := make(chan struct{})
	defer close(stopWait)
	go func() {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			l := s.listener
			s.mu.Unlock()
			if l != nil {
				if err := l.Close(); err != nil {
					fmt.Fprintf(os.Stderr, "failed to close l: %v\n", err)
				}
			}
		case <-stopWait:
		}
	}()

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
				isClosing := (s.listener == nil || s.stopped)
				s.mu.Unlock()
				if isClosing {
					return
				}
				log.Printf("[BranchBase Proxy] Accept error: %v", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(50 * time.Millisecond):
				}
				continue
			}
		}

		select {
		case s.connectionSlots <- struct{}{}:
		default:
			if err := clientConn.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "failed to close clientConn: %v\n", err)
			}
			continue
		}

		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			s.handleConnection(c)
		}(clientConn)
	}
}

func (s *Server) ensureBranchExists(ctx context.Context, targetBranch, defaultBranch string) error {
	if s.drv == nil {
		return nil
	}

	// Fast-path: check in-memory cache without locking or driver query
	s.provisionedMu.RLock()
	if _, ok := s.provisioned[targetBranch]; ok {
		s.provisionedMu.RUnlock()
		return nil
	}
	s.provisionedMu.RUnlock()

	// Slow-path: acquire keyed lock per target branch
	if s.keyLock != nil {
		unlock := s.keyLock.Lock(targetBranch)
		defer unlock()
	}

	// Double-check under lock
	s.provisionedMu.RLock()
	if _, ok := s.provisioned[targetBranch]; ok {
		s.provisionedMu.RUnlock()
		return nil
	}
	s.provisionedMu.RUnlock()

	if ctx == nil {
		ctx = context.Background()
	}
	provCtx, provCancel := context.WithTimeout(ctx, 5*time.Second)
	defer provCancel()
	exists, err := s.drv.BranchExists(provCtx, targetBranch)
	if err != nil {
		return fmt.Errorf("failed to check branch existence for %q: %w", targetBranch, err)
	}
	if !exists {
		log.Printf("[BranchBase Proxy] 🪄 JIT provisioning database for branch %q from %q...", targetBranch, defaultBranch)
		if err := s.drv.CreateBranch(provCtx, defaultBranch, targetBranch); err != nil {
			return fmt.Errorf("failed to JIT provision branch %q: %w", targetBranch, err)
		}
		log.Printf("[BranchBase Proxy] ✅ JIT provisioned database for branch %q", targetBranch)
	}

	s.provisionedMu.Lock()
	s.provisioned[targetBranch] = struct{}{}
	s.provisionedMu.Unlock()
	return nil
}

func (s *Server) sendEarlyError(conn net.Conn, pgCode string, mysqlCode uint16, mysqlState, msg string) {
	if s.cfg != nil && (s.cfg.Driver == "mysql" || s.cfg.Driver == "mariadb") {
		_, _ = conn.Write(mysqlwire.BuildErrorPacket(mysqlCode, mysqlState, msg, 1))
		return
	}
	_, _ = conn.Write(pgwire.BuildErrorResponse("FATAL", pgCode, msg))
}

func (s *Server) dialBackend(ctx context.Context) (net.Conn, string, error) {
	var dialer net.Dialer
	socketPath := s.cfg.Connection.SocketPath
	if socketPath != "" {
		conn, err := dialer.DialContext(ctx, "unix", socketPath)
		return conn, socketPath, err
	}

	host := s.cfg.Connection.Host
	if strings.HasPrefix(host, "/") {
		conn, err := dialer.DialContext(ctx, "unix", host)
		return conn, host, err
	}

	backendAddr := net.JoinHostPort(host, fmt.Sprintf("%d", s.cfg.Connection.Port))
	conn, err := dialer.DialContext(ctx, "tcp", backendAddr)
	return conn, backendAddr, err
}

func (s *Server) handleConnection(clientConn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[BranchBase Proxy] 💥 Recovered from connection panic: %v", r)
			if err := clientConn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				fmt.Fprintf(os.Stderr, "failed to close clientConn after panic: %v\n", err)
			}
		}
	}()
	defer func() { <-s.connectionSlots }()
	rawClientConn := clientConn
	if err := rawClientConn.SetDeadline(time.Now().Add(startupTimeout)); err != nil {
		if err := rawClientConn.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close rawClientConn: %v\n", err)
		}
		return
	}
	s.addActiveConn(rawClientConn)
	defer func() {
		s.removeActiveConn(rawClientConn)
		if err := rawClientConn.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close rawClientConn: %v\n", err)
		}
	}()

	// 1. Resolve active Git branch (Fail-Closed)
	activeBranch, err := git.ResolveCurrentBranch(s.repoPath)
	if err != nil {
		log.Printf("[BranchBase Proxy] ❌ Failed to resolve active Git branch: %v", err)
		errMsg := fmt.Sprintf("BranchBase: refusing connection because active Git branch could not be verified (%v).", err)
		s.sendEarlyError(clientConn, "3D000", 1049, "42000", errMsg)
		return
	}
	defaultBranch := s.cfg.Proxy.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	if activeBranch != "" {
		if err := s.validateBranchIdentity(activeBranch); err != nil {
			log.Printf("[BranchBase Proxy] ❌ Branch database identity is ambiguous: %v", err)
			s.sendEarlyError(clientConn, "3D000", 1049, "42000", err.Error())
			return
		}
	}
	sanitizedBranch := git.SanitizeBranchName(activeBranch)
	targetDB := s.cfg.DatabaseNameForBranch(sanitizedBranch)

	connCtx, connCancel := context.WithTimeout(context.Background(), startupTimeout)
	defer connCancel()

	// 1b. JIT branch provisioning
	if s.drv != nil && sanitizedBranch != git.SanitizeBranchName(defaultBranch) {
		if err := s.ensureBranchExists(connCtx, sanitizedBranch, defaultBranch); err != nil {
			log.Printf("[BranchBase Proxy] ❌ JIT branch provisioning failed for %q: %v", sanitizedBranch, err)
			errMsg := fmt.Sprintf("BranchBase: failed to provision database for branch %q: %v", sanitizedBranch, err)
			s.sendEarlyError(clientConn, "3D000", 1049, "42000", errMsg)
			return
		}
	}

	log.Printf("[BranchBase Proxy] Routing client connection -> Branch: %q (DB: %q)", activeBranch, targetDB)

	// 2. Connect to backend database server
	dialCtx, dialCancel := context.WithTimeout(connCtx, 2*time.Second)
	backendConn, backendAddr, err := s.dialBackend(dialCtx)
	dialCancel()
	if err != nil {
		log.Printf("[BranchBase Proxy] ❌ Failed to connect to backend %s: %v", backendAddr, err)
		errMsg := fmt.Sprintf("BranchBase: failed to connect to database backend %s: %v", backendAddr, err)
		s.sendEarlyError(clientConn, "08006", 2002, "HY000", errMsg)
		return
	}
	s.addActiveConn(backendConn)
	defer func() {
		s.removeActiveConn(backendConn)
		if err := backendConn.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close backendConn: %v\n", err)
		}
	}()

	// 3. Database Wire Protocol Handshake Inspection & Rewriting
	if s.cfg != nil && (s.cfg.Driver == "mysql" || s.cfg.Driver == "mariadb") {
		// In MySQL, the server initiates the handshake by sending Initial Handshake Packet
		_, err := mysqlwire.ForwardPacket(clientConn, backendConn)
		if err != nil {
			log.Printf("[BranchBase Proxy] Error forwarding MySQL initial handshake to client: %v", err)
			return
		}

		// Client responds with HandshakeResponse41
		clientPayload, clientSeqID, err := mysqlwire.ReadPacket(clientConn)
		if err != nil {
			log.Printf("[BranchBase Proxy] Error reading client HandshakeResponse: %v", err)
			return
		}

		// Check if client requested SSL
		if mysqlwire.IsSSLRequest(clientPayload) {
			log.Printf("[BranchBase Proxy] ❌ MySQL SSLRequest is not supported through cleartext proxy. Terminating.")
			errPkt := mysqlwire.BuildErrorPacket(2061, "HY000", "BranchBase: MySQL TLS/SSL negotiation is not supported on transparent proxy; please set sslmode=disable or use unencrypted local connection", clientSeqID+1)
			_, _ = clientConn.Write(errPkt)
			return
		}

		// Rewrite database to targetDB
		rewritten, err := mysqlwire.RewriteHandshakeResponseDatabase(clientPayload, targetDB)
		if err != nil {
			log.Printf("[BranchBase Proxy] ❌ Failed to rewrite MySQL HandshakeResponse database to %q: %v. Aborting connection.", targetDB, err)
			errPkt := mysqlwire.BuildErrorPacket(1049, "42000", fmt.Sprintf("BranchBase: failed to route database to %q: %v", targetDB, err), clientSeqID+1)
			_, _ = clientConn.Write(errPkt)
			return // Strict fail-closed
		}

		if err := mysqlwire.WritePacket(backendConn, rewritten, clientSeqID); err != nil {
			log.Printf("[BranchBase Proxy] Error forwarding rewritten HandshakeResponse to backend: %v", err)
			return
		}

		_ = clientConn.SetDeadline(time.Time{})
		_ = backendConn.SetDeadline(time.Time{})

		s.pipeConnections(clientConn, backendConn)
		return
	}

	// PostgreSQL Wire Protocol Handshake Inspection & Rewriting
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

	// Handle GSSAPI encryption negotiation before SSL or startup processing
	if pgwire.IsGSSENCRequest(packet) {
		if _, err := clientConn.Write([]byte{'N'}); err != nil {
			log.Printf("[BranchBase Proxy] Error responding 'N' to GSSENC request: %v", err)
			return
		}
		packet, err = pgwire.ReadStartupPacket(clientConn)
		if err != nil {
			log.Printf("[BranchBase Proxy] Error reading startup packet after GSSENC negotiation: %v", err)
			return
		}
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
			s.addActiveConn(tlsConn)
			defer func() {
				s.removeActiveConn(tlsConn)
				if err := tlsConn.Close(); err != nil {
					fmt.Fprintf(os.Stderr, "failed to close tlsConn: %v\n", err)
				}
			}()
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
	if err := clientConn.SetDeadline(time.Time{}); err != nil {
		return
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
	s.pipeConnections(clientConn, backendConn)
}

func (s *Server) pipeConnections(clientConn, backendConn net.Conn) {
	errc := make(chan error, 2)
	go func() {
		_, err := io.Copy(backendConn, clientConn)
		errc <- err
	}()
	go func() {
		_, err := io.Copy(clientConn, backendConn)
		errc <- err
	}()

	<-errc
	if err := clientConn.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to close clientConn: %v\n", err)
	}
	if err := backendConn.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to close backendConn: %v\n", err)
	}
	<-errc
}
