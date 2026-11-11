package lnd

import (
	"errors"
	"math"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightningnetwork/lnd/zpay32"

	"github.com/rupeshkoushik07/ln-payment-go/internal/httpapi"
)

// Decoder reads an invoice text ourselves. It calls no node and needs no key:
// an invoice is a signed, public piece of text.
type Decoder struct {
	Net *chaincfg.Params // which network we accept: regtest here, mainnet in production
	Now func() time.Time // the clock; replaceable in tests
}

func NewRegtestDecoder() *Decoder {
	return &Decoder{Net: &chaincfg.RegressionNetParams, Now: time.Now}
}

func (d *Decoder) Decode(text string) (httpapi.Invoice, error) {
	// Checks the format, the network prefix (lnbcrt...) and the signature.
	inv, err := zpay32.Decode(text, d.Net)
	if err != nil {
		return httpapi.Invoice{}, err
	}
	if inv.PaymentHash == nil {
		return httpapi.Invoice{}, errors.New("invoice has no payment hash")
	}

	out := httpapi.Invoice{
		PaymentHash: inv.PaymentHash[:],
		Expired:     d.Now().After(inv.Timestamp.Add(inv.Expiry())),
	}

	// The amount is written in thousandths of a satoshi. We only pay whole sats.
	// An invoice with no amount stays at 0, and the API refuses it.
	if inv.MilliSat != nil {
		msat := uint64(*inv.MilliSat)
		if msat > math.MaxInt64 {
			return httpapi.Invoice{}, errors.New("invoice amount is impossibly large")
		}
		if msat%1000 != 0 {
			return httpapi.Invoice{}, errors.New("invoice amount is not a whole number of sats")
		}
		out.AmountSat = int64(msat / 1000)
	}
	return out, nil
}

// Compile-time check: Decoder is what the pay endpoint asks for.
var _ httpapi.Decoder = (*Decoder)(nil)
