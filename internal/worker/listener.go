package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/rupeshkoushik07/lightning-payments-service/internal/store"
)

// InvoiceEvent is one message from the node about an invoice.
type InvoiceEvent struct {
	Invoice     store.NewInvoice
	AddIndex    uint64
	Paid        bool
	SettleIndex uint64
	PaidAt      time.Time
}

type InvoiceStream interface {
	Subscribe(ctx context.Context, from store.Place, handle func(InvoiceEvent) error) error
}

type InvoiceBook interface {
	ListenerPlace(ctx context.Context) (store.Place, error)
	InvoiceSeen(ctx context.Context, inv store.NewInvoice, addIndex uint64) error
	InvoicePaid(ctx context.Context, paymentHash []byte, settleIndex uint64, paidAt time.Time) error
}

// Listener keeps one line open to the receiving node and records what it says.
type Listener struct {
	Node     InvoiceStream
	Invoices InvoiceBook
	Log      *slog.Logger
}

// Run listens until the program stops. Whenever the line breaks it reconnects
// and asks the node to replay everything after the place we saved.
func (l *Listener) Run(ctx context.Context) {
	for ctx.Err() == nil {
		place, err := l.Invoices.ListenerPlace(ctx)
		if err == nil {
			err = l.Node.Subscribe(ctx, place, func(ev InvoiceEvent) error {
				// Make sure we have the invoice at all, then mark it paid if it is.
				if err := l.Invoices.InvoiceSeen(ctx, ev.Invoice, ev.AddIndex); err != nil {
					return err
				}
				if ev.Paid {
					return l.Invoices.InvoicePaid(ctx, ev.Invoice.PaymentHash, ev.SettleIndex, ev.PaidAt)
				}
				return nil
			})
		}
		if ctx.Err() != nil {
			return // we are shutting down
		}
		l.Log.Warn("invoice line to node broke; reconnecting", "err", err)
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
		}
	}
}
