package store

import "context"

// PayoutsPaused reports whether an operator has stopped outgoing payments.
// It is read from the database every time, so every copy of the service
// sees the same answer and a restart does not forget it.
func (s *Store) PayoutsPaused(ctx context.Context) (bool, error) {
	var paused bool
	err := s.db.QueryRow(ctx, `SELECT payouts_paused FROM settings`).Scan(&paused)
	return paused, err
}

func (s *Store) SetPayoutsPaused(ctx context.Context, paused bool) error {
	_, err := s.db.Exec(ctx, `UPDATE settings SET payouts_paused = $1, updated_at = now()`, paused)
	return err
}

// ExpireInvoices marks open invoices whose time has passed, and returns how many.
// The extra minute gives a last-second payment time to be reported;
// if one still arrives later, PAID wins.
func (s *Store) ExpireInvoices(ctx context.Context) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE invoices SET status = 'EXPIRED'
		WHERE  status = 'OPEN' AND expires_at < now() - interval '1 minute'`)
	return tag.RowsAffected(), err
}
