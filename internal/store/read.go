package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrNotFound = errors.New("not found")

// PaymentView is what the payer's page sees when it polls.
type PaymentView struct {
	ID            int64
	PaymentHash   []byte
	AmountSat     int64
	Status        string
	Preimage      []byte // the receipt; empty until SUCCEEDED
	FailureReason *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (s *Store) GetPayment(ctx context.Context, id int64) (PaymentView, error) {
	var p PaymentView
	err := s.db.QueryRow(ctx, `
		SELECT id, payment_hash, amount_sat, status, preimage, failure_reason, created_at, updated_at
		FROM   payments WHERE id = $1`, id).
		Scan(&p.ID, &p.PaymentHash, &p.AmountSat, &p.Status, &p.Preimage, &p.FailureReason, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// InvoiceView is what the receiver's page sees when it polls.
type InvoiceView struct {
	PaymentHash []byte
	Text        string
	AmountSat   int64
	Status      string
	ExpiresAt   time.Time
	PaidAt      *time.Time
}

func (s *Store) GetInvoice(ctx context.Context, paymentHash []byte) (InvoiceView, error) {
	var v InvoiceView
	err := s.db.QueryRow(ctx, `
		SELECT payment_hash, invoice, amount_sat, status, expires_at, paid_at
		FROM   invoices WHERE payment_hash = $1`, paymentHash).
		Scan(&v.PaymentHash, &v.Text, &v.AmountSat, &v.Status, &v.ExpiresAt, &v.PaidAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}
