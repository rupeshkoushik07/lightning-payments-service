package httpapi

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/rupeshkoushik07/lightning-payments-service/internal/store"
)

type InvoiceMaker interface {
	AddInvoice(ctx context.Context, amountSat int64, memo string, validFor time.Duration) (store.NewInvoice, error)
}
type InvoiceSaver interface {
	SaveInvoice(ctx context.Context, inv store.NewInvoice) error
}

type InvoiceHandler struct {
	Node         InvoiceMaker
	Invoices     InvoiceSaver
	MaxAmountSat int64
	ValidFor     time.Duration // how long an invoice can be paid, for example one hour
}

// POST /invoices   body: {"amount_sat": 1000, "memo": "t-shirt"}
func (h *InvoiceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AmountSat int64  `json:"amount_sat"`
		Memo      string `json:"memo"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		reply(w, http.StatusBadRequest, "body must be JSON with amount_sat")
		return
	}
	if body.AmountSat <= 0 || body.AmountSat > h.MaxAmountSat {
		reply(w, http.StatusUnprocessableEntity, "amount must be positive and within the limit")
		return
	}
	if len(body.Memo) > 200 {
		reply(w, http.StatusUnprocessableEntity, "memo is too long")
		return
	}

	// 1. The node creates the invoice. No money can move from this call.
	inv, err := h.Node.AddInvoice(r.Context(), body.AmountSat, body.Memo, h.ValidFor)
	if err != nil {
		reply(w, http.StatusBadGateway, "the node could not create the invoice")
		return
	}

	// 2. Record it. If this fails the invoice still exists in the node, and the
	//    listener will record it when the node reports it. So we still answer.
	if err := h.Invoices.SaveInvoice(r.Context(), inv); err != nil {
		slog.Error("invoice created in node but not saved; the listener will pick it up",
			"hash", hex.EncodeToString(inv.PaymentHash), "err", err)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"payment_hash": hex.EncodeToString(inv.PaymentHash),
		"invoice":      inv.Text,
		"amount_sat":   inv.AmountSat,
		"expires_at":   inv.ExpiresAt.UTC().Format(time.RFC3339),
		"status":       "OPEN",
	})
}
