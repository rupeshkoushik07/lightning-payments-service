package store

import (
	"context"
	"time"
)

// Place is how far we have read in the node's two numbered lists.
type Place struct {
	AddIndex    uint64
	SettleIndex uint64
}

func (s *Store) ListenerPlace(ctx context.Context) (Place, error) {
	var p Place
	err := s.db.QueryRow(ctx, `SELECT add_index, settle_index FROM listener_state`).
		Scan(&p.AddIndex, &p.SettleIndex)
	return p, err
}

// InvoiceSeen records an invoice the node told us about, and moves our place
// forward, in ONE transaction. Covers invoices the API failed to save.
func (s *Store) InvoiceSeen(ctx context.Context, inv NewInvoice, addIndex uint64) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // does nothing once Commit has succeeded

	if _, err := tx.Exec(ctx, `
		INSERT INTO invoices (payment_hash, invoice, amount_sat, memo, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (payment_hash) DO NOTHING`,
		inv.PaymentHash, inv.Text, inv.AmountSat, inv.Memo, inv.ExpiresAt); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE listener_state SET add_index = $1 WHERE add_index < $1`, addIndex); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// InvoicePaid marks an invoice PAID and moves our place forward, in ONE
// transaction: both happen or neither does.
func (s *Store) InvoicePaid(ctx context.Context, paymentHash []byte, settleIndex uint64, paidAt time.Time) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// "status <> 'PAID'" makes a repeat harmless, and lets PAID win over EXPIRED.
	if _, err := tx.Exec(ctx, `
		UPDATE invoices SET status = 'PAID', paid_at = $2
		WHERE  payment_hash = $1 AND status <> 'PAID'`, paymentHash, paidAt); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE listener_state SET settle_index = $1 WHERE settle_index < $1`, settleIndex); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
