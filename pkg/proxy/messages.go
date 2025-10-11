package proxy

import "github.com/libp2p/go-libp2p/core/protocol"

const (
	ProxyProtocolID = protocol.ID("/bethrou/proxy/1.0.0")
	PingProtocolID  = protocol.ID("/bethrou/ping/1.0.0")
)

type Request struct {
	ProxyAddress string `json:"address"`
}

type ProxyResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}
