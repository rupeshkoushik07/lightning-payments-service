package store

import (
	"context"
	"time"
)

// NewInvoice is an invoice the node has just created for us.
type NewInvoice struct {
	PaymentHash []byte
	Text        string
	AmountSat   int64
	Memo        string
	ExpiresAt   time.Time
}

// SaveInvoice records an invoice. Saving the same one twice is harmless:
// the second attempt hits the unique payment hash and does nothing.
func (s *Store) SaveInvoice(ctx context.Context, inv NewInvoice) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO invoices (payment_hash, invoice, amount_sat, memo, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (payment_hash) DO NOTHING`,
		inv.PaymentHash, inv.Text, inv.AmountSat, inv.Memo, inv.ExpiresAt)
	return err
}
