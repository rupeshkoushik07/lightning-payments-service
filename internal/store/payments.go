// Package store is the only place that talks to Postgres.
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrAlreadyPaying means a live payment for this invoice already exists.
var ErrAlreadyPaying = errors.New("a payment for this invoice is already in progress or done")

type Store struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Store { return &Store{db: db} }

// NewPayment is what the API hands us to save.
type NewPayment struct {
	PaymentHash []byte // 32 bytes, read from the invoice
	Invoice     string
	AmountSat   int64
	FeeLimitSat int64
}

// CreatePayment saves a PENDING row and returns its id.
// We do not check first whether one exists. We just insert, and the
// unique index decides. That is safe even with two requests at once.
func (s *Store) CreatePayment(ctx context.Context, p NewPayment) (int64, error) {
	const q = `
		INSERT INTO payments (payment_hash, invoice, amount_sat, fee_limit_sat)
		VALUES ($1, $2, $3, $4)
		RETURNING id`

	var id int64
	err := s.db.QueryRow(ctx, q, p.PaymentHash, p.Invoice, p.AmountSat, p.FeeLimitSat).Scan(&id)

	// 23505 is Postgres's code for "unique rule broken".
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return 0, ErrAlreadyPaying
	}
	if err != nil {
		return 0, err
	}
	return id, nil
}
