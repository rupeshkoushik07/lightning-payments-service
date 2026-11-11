package lnd

import (
	"context"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"
	"google.golang.org/grpc"

	"github.com/rupeshkoushik07/ln-payment-go/internal/store"
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
