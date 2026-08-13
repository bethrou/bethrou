package socks

import (
	"context"
	"net"
	"time"

	"github.com/bethrou/bethrou/internal/config"
	"github.com/bethrou/bethrou/pkg/logging"
	"github.com/bethrou/bethrou/pkg/proxy"
	"github.com/ezh0v/socks5"
)

type Server struct {
	driver   *Driver
	host     string
	port     int
	internal *socks5.Server
}

func NewServer(ctx context.Context, proxy *proxy.Client, cfg *config.ServerConfig, timeout time.Duration) (*Server, error) {
	host := "127.0.0.1"
	port := 1080

	if cfg.ListenAddr != "" {
		h, p, err := net.SplitHostPort(cfg.ListenAddr)
		if err == nil {
			host = h

			if pp, err := net.LookupPort("tcp", p); err == nil {
				port = pp
			}
		}
	}

	s := &Server{
		driver: &Driver{proxy: proxy, timeout: timeout},
		host:   host,
		port:   port,
	}

	opts := []socks5.Option{
		socks5.WithDriver(s.driver),
		socks5.WithHost(host),
		socks5.WithPort(port),
		socks5.WithLogger(NewLogger(logging.Logger)),
	}

	if cfg.Auth {
		opts = append(opts, socks5.WithPasswordAuthentication())

		opts = append(opts, socks5.WithStaticCredentials(map[string]string{cfg.User: cfg.Pass}))
	}

	s.internal = socks5.New(opts...)

	return s, nil
}

func (s *Server) ListenAndServe() error {
	return s.internal.ListenAndServe()
}

// Shutdown stops accepting new connections and blocks until ListenAndServe
// returns.
func (s *Server) Shutdown() error {
	return s.internal.Shutdown()
}
