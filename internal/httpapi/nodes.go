package httpapi

import (
	"context"
	"net/http"
)

type ReceiverHealth interface {
	Ping(ctx context.Context) error
}
type PayerHealth interface {
	CanSendSat(ctx context.Context) (int64, error)
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
	if sat, err := n.Payer.CanSendSat(r.Context()); err != nil {
		bob["connected"] = false
		bob["problem"] = "node unreachable or key refused"
	} else {
		bob["connected"] = true
		bob["can_send_sat"] = sat
	}
	writeJSON(w, http.StatusOK, []map[string]any{alice, bob})
}
