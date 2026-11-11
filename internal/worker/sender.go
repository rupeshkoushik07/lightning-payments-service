// Package worker holds the background jobs.
package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/rupeshkoushik07/ln-payment-go/internal/store"
)

// Outcome is the node's answer about one payment.
type Outcome int

const (
	Unknown   Outcome = iota // no final answer (line dropped, still trying, timeout)
	Succeeded                // final: we have the preimage
	Failed                   // final: the node says no money moved
	NotFound                 // the node has never heard of this payment hash
)

type Result struct {
	Outcome  Outcome
	Preimage []byte
	Reason   string
}

// Node is bob's LND node, as far as the sender is concerned.
type Node interface {
	Pay(ctx context.Context, job store.Job) (Result, error)
	Track(ctx context.Context, paymentHash []byte) (Result, error)
}

type Payments interface {
	ClaimPending(ctx context.Context) (store.Job, error)
	MarkSucceeded(ctx context.Context, id int64, preimage []byte) error
	MarkFailed(ctx context.Context, id int64, reason string) error
}

type Sender struct {
	Node     Node
	Payments Payments
	Log      *slog.Logger
}

// PayOne takes one pending payment and asks the node to pay it.
func (s *Sender) PayOne(ctx context.Context) error {
	// 1. Take a payment. The row is IN_FLIGHT from this point on.
	job, err := s.Payments.ClaimPending(ctx)
	if err != nil {
		return err // includes store.ErrNothingToDo
	}

	// 2. Ask the node to pay.
	res, err := s.Node.Pay(ctx, job)

	// 3. Final answers are saved even if we are shutting down.
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	switch {
	case err != nil || res.Outcome == Unknown || res.Outcome == NotFound:
		// We do not know. Leave the row IN_FLIGHT; the tracker will ask the node.
		s.Log.Warn("payment outcome unknown, left in flight", "id", job.ID, "err", err)
		return nil
	case res.Outcome == Succeeded:
		return s.Payments.MarkSucceeded(saveCtx, job.ID, res.Preimage)
	default: // Failed
		return s.Payments.MarkFailed(saveCtx, job.ID, res.Reason)
	}
}
