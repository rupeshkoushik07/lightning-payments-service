package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ErrNothingToDo means there is no PENDING payment waiting.
var ErrNothingToDo = errors.New("no pending payment")

// Job is one payment a sender worker has taken to pay.
type Job struct {
	ID          int64
	PaymentHash []byte
	Invoice     string
	AmountSat   int64
	FeeLimitSat int64
}

// ClaimPending takes the oldest PENDING payment, marks it IN_FLIGHT and
// returns it. Picking and marking happen in ONE statement, so two workers
// can never take the same payment. This runs before the node is called.
func (s *Store) ClaimPending(ctx context.Context) (Job, error) {
	const q = `
		UPDATE payments
		SET    status = 'IN_FLIGHT', updated_at = now()
		WHERE  id = (
			SELECT id FROM payments
			WHERE  status = 'PENDING'
			ORDER  BY created_at
			LIMIT  1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, payment_hash, invoice, amount_sat, fee_limit_sat`

	var j Job
	err := s.db.QueryRow(ctx, q).Scan(&j.ID, &j.PaymentHash, &j.Invoice, &j.AmountSat, &j.FeeLimitSat)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNothingToDo
	}
	return j, err
}
