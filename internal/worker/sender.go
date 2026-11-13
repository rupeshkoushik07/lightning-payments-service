// Package worker holds the background jobs.
package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/rupeshkoushik07/lightning-payments-service/internal/metrics"
	"github.com/rupeshkoushik07/lightning-payments-service/internal/store"
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
	FeeSat   int64 // routing fee paid; set only on success
	Reason   string
}

// Node is bob's LND node, as far as the sender is concerned.
type Node interface {
	Pay(ctx context.Context, job store.Job) (Result, error)
	Track(ctx context.Context, paymentHash []byte) (Result, error)
}

type Payments interface {
	PayoutsPaused(ctx context.Context) (bool, error)
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
	// 0. Has an operator paused payouts? Then take nothing; rows wait as PENDING.
	//    If we cannot read the switch, we also do not pay.
	if paused, err := s.Payments.PayoutsPaused(ctx); err != nil || paused {
		return store.ErrNothingToDo
	}

	// 1. Take a payment. The row is IN_FLIGHT from this point on.
	job, err := s.Payments.ClaimPending(ctx)
	if err != nil {
		return err // includes store.ErrNothingToDo
	}

	// 2. Ask the node to pay.
	started := time.Now()
	res, err := s.Node.Pay(ctx, job)
	metrics.PayDuration.Observe(time.Since(started).Seconds())
	metrics.NodeCalls.WithLabelValues("pay", metrics.Result(err)).Inc()

	// 3. Final answers are saved even if we are shutting down.
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	switch {
	case err != nil || res.Outcome == Unknown || res.Outcome == NotFound:
		// We do not know. Leave the row IN_FLIGHT; the tracker will ask the node.
		s.Log.Warn("payment outcome unknown, left in flight", "id", job.ID, "err", err)
		metrics.PaymentsUnknown.Inc()
		return nil
	case res.Outcome == Succeeded:
		err = s.Payments.MarkSucceeded(saveCtx, job.ID, res.Preimage)
		countFinished(err, "SUCCEEDED", "sender")
		if err == nil {
			metrics.FeesPaidSat.Add(float64(res.FeeSat))
			if job.FeeLimitSat > 0 {
				metrics.FeeLimitUsed.Observe(float64(res.FeeSat) / float64(job.FeeLimitSat))
			}
		}
		return err
	default: // Failed
		err = s.Payments.MarkFailed(saveCtx, job.ID, res.Reason)
		countFinished(err, "FAILED", "sender")
		return err
	}
}

// countFinished counts a final status only once it is saved.
func countFinished(err error, status, by string) {
	if err == nil {
		metrics.PaymentsFinished.WithLabelValues(status, by).Inc()
	}
}
