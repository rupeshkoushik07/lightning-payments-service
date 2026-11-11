package httpapi

import (
	"context"
	"net/http"
)

// Sendable describes what the paying node can send right now.
type Sendable struct {
	ActiveChannels int   // channels whose partner is connected: usable
	TotalChannels  int   // all open channels, usable or not
	Sat            int64 // our side of the usable channels
}

type ReceiverHealth interface {
	Ping(ctx context.Context) error
}
type PayerHealth interface {
	CanSend(ctx context.Context) (Sendable, error)
}

// Nodes reports whether each Lightning node is reachable with the key we hold.
// It shows hosts, never keys.
type Nodes struct {
	Receiver     ReceiverHealth
	ReceiverHost string
	Payer        PayerHealth
	PayerHost    string
}

// GET /api/nodes
func (n *Nodes) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	alice := map[string]any{"name": "alice", "job": "receives", "host": n.ReceiverHost, "key": "invoice macaroon (cannot spend)"}
	if err := n.Receiver.Ping(r.Context()); err != nil {
		alice["connected"] = false
		alice["problem"] = "node unreachable or key refused"
	} else {
		alice["connected"] = true
	}

	bob := map[string]any{"name": "bob", "job": "pays", "host": n.PayerHost, "key": "payment macaroon"}
	if s, err := n.Payer.CanSend(r.Context()); err != nil {
		bob["connected"] = false
		bob["problem"] = "node unreachable or key refused"
	} else {
		bob["connected"] = true
		bob["can_send_sat"] = s.Sat
		bob["active_channels"] = s.ActiveChannels
		bob["total_channels"] = s.TotalChannels
	}
	writeJSON(w, http.StatusOK, []map[string]any{alice, bob})
}
