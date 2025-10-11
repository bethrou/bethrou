package socks

import (
	"context"
	"net"

	"github.com/ezh0v/socks5"
	"github.com/henrybarreto/bethrou/client/config"
	"github.com/henrybarreto/bethrou/pkg/logging"
	"github.com/henrybarreto/bethrou/pkg/proxy"
)

type Server struct {
	driver   *Driver
	host     string
	port     int
	internal *socks5.Server
}

func NewServer(ctx context.Context, proxy *proxy.Client, cfg *config.ServerConfig) (*Server, error) {
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
		driver: &Driver{proxy: proxy},
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
