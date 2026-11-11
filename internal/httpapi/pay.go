// Package httpapi turns HTTP requests into calls on the store.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/rupeshkoushik07/lightning-payments-service/internal/store"
)

// Invoice is what we read out of the invoice text.
type Invoice struct {
	PaymentHash []byte
	AmountSat   int64
	Expired     bool
}

// The handler only needs these two abilities. It does not care how they are done.
type Decoder interface {
	Decode(invoice string) (Invoice, error)
}
type PaymentSaver interface {
	CreatePayment(ctx context.Context, p store.NewPayment) (int64, error)
}

type PauseChecker interface {
	PayoutsPaused(ctx context.Context) (bool, error)
}

type PayHandler struct {
	Pause        PauseChecker
	Decoder      Decoder
	Payments     PaymentSaver
	MaxAmountSat int64
}

// POST /payments   body: {"invoice": "lnbc..."}
func (h *PayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Invoice string `json:"invoice"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096) // an invoice is short; refuse huge bodies
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		reply(w, http.StatusBadRequest, "body must be JSON with an invoice field")
		return
	}

	// 0. Kill switch. If we cannot read it, we refuse too: fail closed.
	if paused, err := h.Pause.PayoutsPaused(r.Context()); err != nil || paused {
		reply(w, http.StatusServiceUnavailable, "payouts are paused, try again later")
		return
	}

	// 1. Read the invoice ourselves. No call to any node.
	inv, err := h.Decoder.Decode(body.Invoice)
	if err != nil {
		reply(w, http.StatusBadRequest, "not a valid invoice")
		return
	}

	// 2. Our own checks, before anything is saved.
	if inv.Expired {
		reply(w, http.StatusUnprocessableEntity, "invoice has expired")
		return
	}
	if inv.AmountSat <= 0 || inv.AmountSat > h.MaxAmountSat {
		reply(w, http.StatusUnprocessableEntity, "amount is missing or over the limit")
		return
	}

	// 3. Save first. The database decides whether this is a duplicate.
	id, err := h.Payments.CreatePayment(r.Context(), store.NewPayment{
		PaymentHash: inv.PaymentHash,
		Invoice:     body.Invoice,
		AmountSat:   inv.AmountSat,
		FeeLimitSat: inv.AmountSat/100 + 10, // at most 1% plus 10 sats in routing fees
	})
	if errors.Is(err, store.ErrAlreadyPaying) {
		reply(w, http.StatusConflict, "this invoice is already being paid or was paid")
		return
	}
	if err != nil {
		reply(w, http.StatusInternalServerError, "could not save the payment")
		return
	}

	// 4. Answer now. A worker pays in the background.
	writeJSON(w, http.StatusAccepted, map[string]any{"id": id, "status": "PENDING"})
}

func reply(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// writeJSON sends the answer. If writing fails the client has gone away;
// we log it, because there is no one left to tell.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("could not write response", "err", err)
	}
}
