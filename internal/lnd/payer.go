package lnd

import (
	"context"
	"encoding/hex"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/rupeshkoushik07/lightning-payments-service/internal/httpapi"
	"github.com/rupeshkoushik07/lightning-payments-service/internal/store"
	"github.com/rupeshkoushik07/lightning-payments-service/internal/worker"
)

// Payer gives orders to the paying node (bob).
type Payer struct {
	router routerrpc.RouterClient
	ln     lnrpc.LightningClient
}

func NewPayer(conn *grpc.ClientConn) *Payer {
	return &Payer{router: routerrpc.NewRouterClient(conn), ln: lnrpc.NewLightningClient(conn)}
}

// CanSend asks the node which of its channels are usable. A channel is open
// on the blockchain all the time, but it can only carry a payment while the
// two nodes are connected to each other ("active").
// It doubles as a health check: an answer means the node is up and accepts our key.
func (p *Payer) CanSend(ctx context.Context) (httpapi.Sendable, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	res, err := p.ln.ListChannels(ctx, &lnrpc.ListChannelsRequest{})
	if err != nil {
		return httpapi.Sendable{}, err
	}
	var s httpapi.Sendable
	for _, ch := range res.Channels {
		s.TotalChannels++
		if ch.Active {
			s.ActiveChannels++
			s.Sat += ch.LocalBalance
		}
	}
	return s, nil
}

// Pay is call 1: "pay this invoice". It waits for the node's final answer.
func (p *Payer) Pay(ctx context.Context, job store.Job) (worker.Result, error) {
	stream, err := p.router.SendPaymentV2(ctx, &routerrpc.SendPaymentRequest{
		PaymentRequest:    job.Invoice,
		FeeLimitSat:       job.FeeLimitSat, // never pay more than this in routing fees
		TimeoutSeconds:    60,              // stop looking for routes after a minute
		NoInflightUpdates: true,            // tell us only the final result
	})
	if err != nil {
		return worker.Result{}, err // the call had a problem; the payment is unknown
	}
	pay, err := stream.Recv()
	if err != nil {
		return worker.Result{}, err // line dropped while waiting; unknown
	}
	return toResult(pay), nil
}

// Track is call 2: "what happened to the payment with this hash?"
// It only asks. It can never move money.
func (p *Payer) Track(ctx context.Context, paymentHash []byte) (worker.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	stream, err := p.router.TrackPaymentV2(ctx, &routerrpc.TrackPaymentRequest{
		PaymentHash: paymentHash,
	})
	if err != nil {
		return worker.Result{}, err
	}
	pay, err := stream.Recv() // the first message is the payment's state right now
	if status.Code(err) == codes.NotFound {
		return worker.Result{Outcome: worker.NotFound}, nil
	}
	if err != nil {
		return worker.Result{}, err
	}
	return toResult(pay), nil
}

// toResult turns the node's answer into our three outcomes.
func toResult(pay *lnrpc.Payment) worker.Result {
	switch pay.Status {
	case lnrpc.Payment_SUCCEEDED:
		preimage, err := hex.DecodeString(pay.PaymentPreimage)
		if err != nil || len(preimage) != 32 {
			return worker.Result{Outcome: worker.Unknown} // no valid receipt, no success
		}
		return worker.Result{Outcome: worker.Succeeded, Preimage: preimage}
	case lnrpc.Payment_FAILED:
		return worker.Result{Outcome: worker.Failed, Reason: pay.FailureReason.String()}
	default: // still in flight
		return worker.Result{Outcome: worker.Unknown}
	}
}

// Compile-time check: Payer can do everything the workers need from a node.
var _ worker.Node = (*Payer)(nil)
