package lnd

import (
	"bytes"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/zpay32"
)

// makeInvoice builds a real, signed invoice the way a node would.
func makeInvoice(t *testing.T, net *chaincfg.Params, hash [32]byte, msat int64, created time.Time, validFor time.Duration) string {
	t.Helper()
	key, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	inv, err := zpay32.NewInvoice(net, hash, created,
		zpay32.Amount(lnwire.MilliSatoshi(msat)),
		zpay32.Description("test"),
		zpay32.Expiry(validFor),
	)
	if err != nil {
		t.Fatal(err)
	}
	text, err := inv.Encode(zpay32.MessageSigner{
		SignCompact: func(msg []byte) ([]byte, error) {
			return ecdsa.SignCompact(key, chainhash.HashB(msg), true), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func TestDecode(t *testing.T) {
	hash := sha256.Sum256([]byte("a secret only alice knows"))
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	d := &Decoder{Net: &chaincfg.RegressionNetParams, Now: func() time.Time { return now }}

	t.Run("reads hash and amount", func(t *testing.T) {
		got, err := d.Decode(makeInvoice(t, d.Net, hash, 1_000_000, now, time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got.PaymentHash, hash[:]) || got.AmountSat != 1000 || got.Expired {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("notices an expired invoice", func(t *testing.T) {
		got, err := d.Decode(makeInvoice(t, d.Net, hash, 1_000_000, now.Add(-2*time.Hour), time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if !got.Expired {
			t.Error("want Expired")
		}
	})

	t.Run("refuses an invoice from another network", func(t *testing.T) {
		mainnet := makeInvoice(t, &chaincfg.MainNetParams, hash, 1_000_000, now, time.Hour)
		if _, err := d.Decode(mainnet); err == nil {
			t.Error("a mainnet invoice must be refused on regtest")
		}
	})

	t.Run("refuses a fraction of a sat", func(t *testing.T) {
		if _, err := d.Decode(makeInvoice(t, d.Net, hash, 1_500, now, time.Hour)); err == nil {
			t.Error("1.5 sats must be refused")
		}
	})

	t.Run("refuses garbage", func(t *testing.T) {
		if _, err := d.Decode("'); DROP TABLE payments; --"); err == nil {
			t.Error("garbage must be refused")
		}
	})
}
