package httpapi

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/rupeshkoushik07/ln-payment-go/internal/store"
)

type Reader interface {
	GetPayment(ctx context.Context, id int64) (store.PaymentView, error)
	GetInvoice(ctx context.Context, paymentHash []byte) (store.InvoiceView, error)
}

// GET /payments/{id} – the payer's page polls this.
func GetPayment(db Reader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			reply(w, http.StatusBadRequest, "id must be a number")
			return
		}
		p, err := db.GetPayment(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			reply(w, http.StatusNotFound, "no such payment")
			return
		}
		if err != nil {
			reply(w, http.StatusInternalServerError, "could not read the payment")
			return
		}
		out := map[string]any{
			"id":           p.ID,
			"payment_hash": hex.EncodeToString(p.PaymentHash),
			"amount_sat":   p.AmountSat,
			"status":       p.Status,
			"updated_at":   p.UpdatedAt.UTC().Format(time.RFC3339),
		}
		if p.Preimage != nil {
			out["preimage"] = hex.EncodeToString(p.Preimage) // the payer's receipt
		}
		if p.FailureReason != nil {
			out["failure_reason"] = *p.FailureReason
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// GET /invoices/{hash} – the receiver's page polls this.
func GetInvoice(db Reader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hash, err := hex.DecodeString(r.PathValue("hash"))
		if err != nil || len(hash) != 32 {
			reply(w, http.StatusBadRequest, "hash must be 64 hex characters")
			return
		}
		v, err := db.GetInvoice(r.Context(), hash)
		if errors.Is(err, store.ErrNotFound) {
			reply(w, http.StatusNotFound, "no such invoice")
			return
		}
		if err != nil {
			reply(w, http.StatusInternalServerError, "could not read the invoice")
			return
		}
		out := map[string]any{
			"payment_hash": hex.EncodeToString(v.PaymentHash),
			"invoice":      v.Text,
			"amount_sat":   v.AmountSat,
			"status":       v.Status,
			"expires_at":   v.ExpiresAt.UTC().Format(time.RFC3339),
		}
		if v.PaidAt != nil {
			out["paid_at"] = v.PaidAt.UTC().Format(time.RFC3339)
		}
		writeJSON(w, http.StatusOK, out)
	}
}

type PauseSetter interface {
	SetPayoutsPaused(ctx context.Context, paused bool) error
}

// POST /admin/payouts   body: {"paused": true}
// No sign-in yet: the server listens on this machine only. Sign-in comes later.
func SetPause(db PauseSetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Paused *bool `json:"paused"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Paused == nil {
			reply(w, http.StatusBadRequest, `body must be {"paused": true} or {"paused": false}`)
			return
		}
		if err := db.SetPayoutsPaused(r.Context(), *body.Paused); err != nil {
			reply(w, http.StatusInternalServerError, "could not change the switch")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"payouts_paused": *body.Paused})
	}
}
