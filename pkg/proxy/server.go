package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/bethrou/bethrou/pkg/logging"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Server handles incoming proxy requests from clients
type Server struct {
	host         host.Host
	cfg          ServerConfig
	mu           sync.Mutex
	activeTotal  int
	activeByPeer map[peer.ID]int
}

type ServerConfig struct {
	AllowAllPeers               bool
	AllowedPeers                map[peer.ID]struct{}
	DialTimeout                 time.Duration
	MaxConcurrentStreams        int
	MaxConcurrentStreamsPerPeer int
	IdleTimeout                 time.Duration
	DestinationPolicy           DestinationPolicy
}

// NewServer creates a new proxy handler for the server (node) side
func NewServer(h host.Host, cfg ServerConfig) *Server {
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 10 * time.Second
	}
	if cfg.MaxConcurrentStreams <= 0 {
		cfg.MaxConcurrentStreams = 128
	}
	if cfg.MaxConcurrentStreamsPerPeer <= 0 {
		cfg.MaxConcurrentStreamsPerPeer = 8
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 2 * time.Minute
	}

	s := &Server{
		host:         h,
		cfg:          cfg,
		activeByPeer: make(map[peer.ID]int),
	}
	s.host.SetStreamHandler(ProxyProtocolID, s.handle)
	s.host.SetStreamHandler(PingProtocolID, func(s network.Stream) {
		_ = s.Close()
	})

	return s
}

// handle processes an incoming proxy stream
func (h *Server) handle(s network.Stream) {
	defer s.Close()

	remotePeer := s.Conn().RemotePeer()

	logging.Info("New proxy stream", "from", remotePeer)
	if !h.isPeerAllowed(remotePeer) {
		err := fmt.Errorf("peer %s is not authorized to use this exit node", remotePeer)
		logging.Warn("Unauthorized proxy request", "peer", remotePeer)
		h.sendError(s, err)
		return
	}
	if err := h.acquire(remotePeer); err != nil {
		logging.Warn("Rejected proxy stream", "peer", remotePeer, "error", err)
		h.sendError(s, err)
		return
	}
	defer h.release(remotePeer)

	var req Request
	if err := s.SetReadDeadline(time.Now().Add(h.cfg.DialTimeout)); err != nil {
		logging.Warn("Failed to set request read deadline", "error", err)
	}
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		if err == io.EOF {
			logging.Warn("Empty proxy request", "from", remotePeer)
		}

		logging.Error("Failed to decode proxy request", "error", err)
		h.sendError(s, err)

		return
	}

	logging.Info("Proxying to", "addr", req.ProxyAddress)
	dialTimeout := h.effectiveDialTimeout(req.ConnectTimeoutMS)
	resolved, err := h.cfg.DestinationPolicy.Resolve(req.ProxyAddress, dialTimeout)
	if err != nil {
		logging.Warn("Destination blocked by policy", "peer", remotePeer, "addr", req.ProxyAddress, "error", err)
		h.sendError(s, err)
		return
	}

	// Dial the already-validated IP literal rather than re-resolving
	// req.ProxyAddress, so a DNS answer that changes between validation
	// and dial (rebinding) cannot bypass the policy check.
	conn, err := (&net.Dialer{Timeout: dialTimeout}).Dial("tcp", resolved.String())
	if err != nil {
		logging.Error("Failed to connect to proxy address", "addr", req.ProxyAddress, "error", err)
		h.sendError(s, err)
		return
	}

	defer conn.Close()

	if err := h.sendSuccess(s); err != nil {
		logging.Error("Failed to send success response", "error", err)
		return
	}
	if err := s.SetDeadline(time.Time{}); err != nil {
		logging.Warn("Failed to clear stream deadline", "error", err)
	}

	logging.Info("Starting data forwarding", "addr", req.ProxyAddress)

	if err := h.forward(s, conn); err != nil {
		logging.Error("Forwarding error", "error", err)
	}

	logging.Info("Proxy stream completed", "addr", req.ProxyAddress)
}

// sendError sends an error response to the client
func (h *Server) sendError(s network.Stream, err error) {
	resp := ProxyResponse{
		Status:  "error",
		Message: err.Error(),
	}
	if encErr := json.NewEncoder(s).Encode(resp); encErr != nil {
		logging.Error("Failed to encode error response", "error", encErr)
	}
}

// sendSuccess sends a success response to the client
func (h *Server) sendSuccess(s network.Stream) error {
	resp := ProxyResponse{Status: "ok"}
	return json.NewEncoder(s).Encode(resp)
}

// forward bidirectionally forwards data between the stream and the TCP connection
func (h *Server) forward(s network.Stream, conn net.Conn) error {
	errCh := make(chan error, 2)

	go func() {
		errCh <- copyWithIdleTimeout(conn, s, h.cfg.IdleTimeout)
	}()

	go func() {
		errCh <- copyWithIdleTimeout(s, conn, h.cfg.IdleTimeout)
	}()

	err := <-errCh
	if err != nil && err != io.EOF {
		return fmt.Errorf("forwarding failed: %w", err)
	}

	return nil
}

func (s *Server) Listen(ctx context.Context) {
	logging.Info("Server is listening for incoming proxy streams")

	<-ctx.Done()
}

func (s *Server) isPeerAllowed(id peer.ID) bool {
	if s.cfg.AllowAllPeers {
		return true
	}

	_, ok := s.cfg.AllowedPeers[id]
	return ok
}

func (s *Server) effectiveDialTimeout(requestTimeoutMS int64) time.Duration {
	timeout := s.cfg.DialTimeout
	if requestTimeoutMS <= 0 {
		return timeout
	}

	requestTimeout := time.Duration(requestTimeoutMS) * time.Millisecond
	if timeout <= 0 || requestTimeout < timeout {
		return requestTimeout
	}

	return timeout
}

func (s *Server) acquire(id peer.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.activeTotal >= s.cfg.MaxConcurrentStreams {
		return fmt.Errorf("node is at maximum concurrent stream capacity")
	}
	if s.activeByPeer[id] >= s.cfg.MaxConcurrentStreamsPerPeer {
		return fmt.Errorf("peer %s exceeded concurrent stream limit", id)
	}

	s.activeTotal++
	s.activeByPeer[id]++
	return nil
}

func (s *Server) release(id peer.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.activeByPeer[id] > 1 {
		s.activeByPeer[id]--
	} else {
		delete(s.activeByPeer, id)
	}

	if s.activeTotal > 0 {
		s.activeTotal--
	}
}

type idleReadWriter interface {
	io.Reader
	io.Writer
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}

func copyWithIdleTimeout(dst idleReadWriter, src idleReadWriter, idle time.Duration) error {
	buf := make([]byte, 32*1024)

	for {
		if idle > 0 {
			if err := src.SetReadDeadline(time.Now().Add(idle)); err != nil {
				return err
			}
		}

		n, err := src.Read(buf)
		if n > 0 {
			if idle > 0 {
				if err := dst.SetWriteDeadline(time.Now().Add(idle)); err != nil {
					return err
				}
			}

			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				if isTimeout(writeErr) {
					return fmt.Errorf("idle timeout reached while writing: %w", writeErr)
				}
				return writeErr
			}
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				return io.EOF
			}
			if isTimeout(err) {
				return fmt.Errorf("idle timeout reached while reading: %w", err)
			}
			return err
		}
	}
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
