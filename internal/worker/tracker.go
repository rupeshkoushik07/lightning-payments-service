package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/rupeshkoushik07/ln-payment-go/internal/store"
)

// notFoundGrace is how old a row must be before we believe
// "the node has never heard of it".
const notFoundGrace = time.Minute

type OpenPayments interface {
	ListInFlight(ctx context.Context) ([]store.Open, error)
	MarkSucceeded(ctx context.Context, id int64, preimage []byte) error
	MarkFailed(ctx context.Context, id int64, reason string) error
}

type Tracker struct {
	Node     Node
	Payments OpenPayments
	Log      *slog.Logger
}

// ResolveAll asks the node about every payment we are unsure of.
// It runs at startup and then every few seconds. It only asks; it never pays.
func (t *Tracker) ResolveAll(ctx context.Context) error {
	open, err := t.Payments.ListInFlight(ctx)
	if err != nil {
		return err
	}
	for _, p := range open {
		res, err := t.Node.Track(ctx, p.PaymentHash)
		if err != nil {
			// Could not reach the node. Change nothing; try again next round.
			t.Log.Warn("could not ask node about payment", "id", p.ID, "err", err)
			continue
		}
		switch res.Outcome {
		case Succeeded:
			err = t.Payments.MarkSucceeded(ctx, p.ID, res.Preimage)
		case Failed:
			err = t.Payments.MarkFailed(ctx, p.ID, res.Reason)
		case NotFound:
			if p.Age > notFoundGrace {
				err = t.Payments.MarkFailed(ctx, p.ID, "node never received the payment request")
			}
		case Unknown:
			// Still trying. Leave it; ask again next round.
		}
		if err != nil {
			t.Log.Error("could not save payment result", "id", p.ID, "err", err)
		}
	}
	return nil
}
