package httpapi

import (
	"context"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/rupeshkoushik07/ln-payment-go/internal/store"
)

type Lister interface {
	ListPayments(ctx context.Context, limit int) ([]store.PaymentView, error)
	ListInvoices(ctx context.Context, limit int) ([]store.InvoiceView, error)
	PayoutsPaused(ctx context.Context) (bool, error)
}

// GET /api/payments – the latest 20.
func ListPayments(db Lister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.ListPayments(r.Context(), 20)
		if err != nil {
			reply(w, http.StatusInternalServerError, "could not read payments")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, p := range rows {
			m := map[string]any{
				"id": p.ID, "amount_sat": p.AmountSat, "status": p.Status,
				"payment_hash": hex.EncodeToString(p.PaymentHash),
				"updated_at":   p.UpdatedAt.UTC().Format(time.RFC3339),
			}
			if p.Preimage != nil {
				m["preimage"] = hex.EncodeToString(p.Preimage)
			}
			if p.FailureReason != nil {
				m["failure_reason"] = *p.FailureReason
			}
			out = append(out, m)
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// GET /api/invoices – the latest 20.
func ListInvoices(db Lister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.ListInvoices(r.Context(), 20)
		if err != nil {
			reply(w, http.StatusInternalServerError, "could not read invoices")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, v := range rows {
			m := map[string]any{
				"amount_sat": v.AmountSat, "status": v.Status, "invoice": v.Text,
				"payment_hash": hex.EncodeToString(v.PaymentHash),
				"expires_at":   v.ExpiresAt.UTC().Format(time.RFC3339),
			}
			if v.PaidAt != nil {
				m["paid_at"] = v.PaidAt.UTC().Format(time.RFC3339)
			}
			out = append(out, m)
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// GET /api/admin/payouts – is the switch on?
func GetPause(db Lister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		paused, err := db.PayoutsPaused(r.Context())
		if err != nil {
			reply(w, http.StatusInternalServerError, "could not read the switch")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"payouts_paused": paused})
	}
}
