package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"

	"github.com/henrybarreto/bethrou/pkg/logging"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
)

// Server handles incoming proxy requests from clients
type Server struct {
	host host.Host
}

// NewServer creates a new proxy handler for the server (node) side
func NewServer(h host.Host) *Server {
	s := &Server{host: h}
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

	logging.Logger.Info("New proxy stream", "from", remotePeer)

	var req Request
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		if err == io.EOF {
			logging.Logger.Warn("Empty proxy request", "from", remotePeer)
		}

		logging.Logger.Error("Failed to decode proxy request", "error", err)
		h.sendError(s, err)

		return
	}

	logging.Logger.Info("Proxying to", "addr", req.ProxyAddress)

	conn, err := net.Dial("tcp", req.ProxyAddress)
	if err != nil {
		logging.Logger.Error("Failed to connect to proxy address", "addr", req.ProxyAddress, "error", err)
		h.sendError(s, err)
		return
	}

	defer conn.Close()

	if err := h.sendSuccess(s); err != nil {
		logging.Logger.Error("Failed to send success response", "error", err)
		return
	}

	logging.Logger.Info("Starting data forwarding", "addr", req.ProxyAddress)

	if err := h.forward(s, conn); err != nil {
		logging.Logger.Error("Forwarding error", "error", err)
	}

	logging.Logger.Info("Proxy stream completed", "addr", req.ProxyAddress)
}

// sendError sends an error response to the client
func (h *Server) sendError(s network.Stream, err error) {
	resp := ProxyResponse{
		Status:  "error",
		Message: err.Error(),
	}
	if encErr := json.NewEncoder(s).Encode(resp); encErr != nil {
		logging.Logger.Error("Failed to encode error response", "error", encErr)
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
		_, err := io.Copy(conn, s)
		errCh <- err
	}()

	go func() {
		_, err := io.Copy(s, conn)
		errCh <- err
	}()

	err := <-errCh
	if err != nil && err != io.EOF {
		return fmt.Errorf("forwarding failed: %w", err)
	}

	return nil
}

func (s *Server) Listen(ctx context.Context) {
	logging.Logger.Info("Server is listening for incoming proxy streams")

	<-ctx.Done()
}
