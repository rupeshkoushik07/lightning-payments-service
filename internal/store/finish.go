package store

import "context"

// MarkSucceeded closes a payment with its receipt.
// "AND status = 'IN_FLIGHT'" makes it safe to call twice: the second call changes nothing.
func (s *Store) MarkSucceeded(ctx context.Context, id int64, preimage []byte) error {
	_, err := s.db.Exec(ctx, `
		UPDATE payments
		SET    status = 'SUCCEEDED', preimage = $2, updated_at = now()
		WHERE  id = $1 AND status = 'IN_FLIGHT'`, id, preimage)
	return err
}

// MarkFailed closes a payment the node said has failed.
func (s *Store) MarkFailed(ctx context.Context, id int64, reason string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE payments
		SET    status = 'FAILED', failure_reason = $2, updated_at = now()
		WHERE  id = $1 AND status IN ('PENDING', 'IN_FLIGHT')`, id, reason)
	return err
}
