package store

import (
	"context"
	"time"
)

// Open is a payment whose outcome we do not know yet.
type Open struct {
	ID          int64
	PaymentHash []byte
	Age         time.Duration // how long since we handed it to the node
}

// ListInFlight returns every IN_FLIGHT payment, oldest first.
func (s *Store) ListInFlight(ctx context.Context) ([]Open, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, payment_hash, now() - updated_at
		FROM   payments
		WHERE  status = 'IN_FLIGHT'
		ORDER  BY updated_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Open
	for rows.Next() {
		var o Open
		if err := rows.Scan(&o.ID, &o.PaymentHash, &o.Age); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
