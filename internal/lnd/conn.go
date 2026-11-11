// Package lnd is the only place that talks to the Lightning nodes.
package lnd

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// NodeConfig is the three things we need to reach one node.
// The two paths point at files outside the repository.
type NodeConfig struct {
	Host         string // for example 127.0.0.1:10001
	TLSCertPath  string // proves the node is who it says it is
	MacaroonPath string // our key: says what we are allowed to do
}

// Dial opens one long-lived, encrypted connection to a node.
func Dial(cfg NodeConfig) (*grpc.ClientConn, error) {
	// 1. Trust only this node's own certificate (pinned), nothing else.
	tlsCreds, err := credentials.NewClientTLSFromFile(cfg.TLSCertPath, "")
	if err != nil {
		return nil, fmt.Errorf("read TLS cert: %w", err)
	}

	// 2. Load our key once, at startup. It is never logged.
	mac, err := os.ReadFile(cfg.MacaroonPath)
	if err != nil {
		return nil, fmt.Errorf("read macaroon: %w", err)
	}

	// 3. Every call on this connection is encrypted and carries the key.
	return grpc.NewClient(cfg.Host,
		grpc.WithTransportCredentials(tlsCreds),
		grpc.WithPerRPCCredentials(macaroonCreds{hex: hex.EncodeToString(mac)}),
	)
}

// macaroonCreds attaches the macaroon to each call, the way LND expects it.
type macaroonCreds struct{ hex string }

func (m macaroonCreds) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"macaroon": m.hex}, nil
}

// The key may only travel over an encrypted connection.
func (m macaroonCreds) RequireTransportSecurity() bool { return true }
