package lnd

import (
	"context"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"
	"google.golang.org/grpc"

	"github.com/rupeshkoushik07/ln-payment-go/internal/store"
	"github.com/rupeshkoushik07/ln-payment-go/internal/worker"
)

// Receiver gives orders to the receiving node (alice).
// It works with the invoice macaroon, which cannot spend.
type Receiver struct {
	ln lnrpc.LightningClient
}

func NewReceiver(conn *grpc.ClientConn) *Receiver {
	return &Receiver{ln: lnrpc.NewLightningClient(conn)}
}

// AddInvoice asks the node to create an invoice. The node invents the
// secret and keeps it; we only get the hash and the invoice text back.
func (r *Receiver) AddInvoice(ctx context.Context, amountSat int64, memo string, validFor time.Duration) (store.NewInvoice, error) {
	asked := time.Now()
	res, err := r.ln.AddInvoice(ctx, &lnrpc.Invoice{
		Value:  amountSat,
		Memo:   memo,
		Expiry: int64(validFor.Seconds()),
	})
	if err != nil {
		return store.NewInvoice{}, err
	}
	return store.NewInvoice{
		PaymentHash: res.RHash,
		Text:        res.PaymentRequest,
		AmountSat:   amountSat,
		Memo:        memo,
		ExpiresAt:   asked.Add(validFor),
	}, nil
}

// Subscribe opens the long-lived line to the node and calls handle for every
// invoice created or paid after the given place. It returns when the line breaks.
func (r *Receiver) Subscribe(ctx context.Context, from store.Place, handle func(worker.InvoiceEvent) error) error {
	stream, err := r.ln.SubscribeInvoices(ctx, &lnrpc.InvoiceSubscription{
		AddIndex:    from.AddIndex,    // "replay every invoice created after this number"
		SettleIndex: from.SettleIndex, // "replay every invoice paid after this number"
	})
	if err != nil {
		return err
	}
	for {
		inv, err := stream.Recv() // waits here until the node has something to say
		if err != nil {
			return err
		}
		ev := worker.InvoiceEvent{
			Invoice: store.NewInvoice{
				PaymentHash: inv.RHash,
				Text:        inv.PaymentRequest,
				AmountSat:   inv.Value,
				Memo:        inv.Memo,
				ExpiresAt:   time.Unix(inv.CreationDate+inv.Expiry, 0),
			},
			AddIndex:    inv.AddIndex,
			Paid:        inv.State == lnrpc.Invoice_SETTLED,
			SettleIndex: inv.SettleIndex,
			PaidAt:      time.Unix(inv.SettleDate, 0),
		}
		if err := handle(ev); err != nil {
			return err // stop; we will reconnect and the node will replay this one
		}
	}
}
