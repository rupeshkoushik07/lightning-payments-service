package store

import "context"

// Books compares what we sent (payments) with what we received (invoices).
// Both tables live in our database, so the comparison is exact.
type Books struct {
	SentSat     int64 // sum of SUCCEEDED payments
	ReceivedSat int64 // sum of PAID invoices
	// Money left our payer but the matching invoice is not PAID after a minute.
	NotCredited int64
	// Same payment hash, different amount on each side. Must always be 0.
	AmountDiffers int64
	// Invoice PAID with no payment from us (someone paid alice directly). Not an error.
	PaidOutside int64
}

// Reconcile reads both sides in one statement, so they come from the same moment.
func (s *Store) Reconcile(ctx context.Context) (Books, error) {
	var b Books
	err := s.db.QueryRow(ctx, `
		SELECT
		  (SELECT coalesce(sum(amount_sat), 0) FROM payments WHERE status = 'SUCCEEDED'),
		  (SELECT coalesce(sum(amount_sat), 0) FROM invoices WHERE status = 'PAID'),
		  (SELECT count(*) FROM payments p JOIN invoices i USING (payment_hash)
		    WHERE p.status = 'SUCCEEDED' AND i.status <> 'PAID'
		      AND p.updated_at < now() - interval '1 minute'),
		  (SELECT count(*) FROM payments p JOIN invoices i USING (payment_hash)
		    WHERE p.status = 'SUCCEEDED' AND p.amount_sat <> i.amount_sat),
		  (SELECT count(*) FROM invoices i
		    WHERE i.status = 'PAID' AND NOT EXISTS (
		      SELECT 1 FROM payments p WHERE p.payment_hash = i.payment_hash AND p.status = 'SUCCEEDED'))`,
	).Scan(&b.SentSat, &b.ReceivedSat, &b.NotCredited, &b.AmountDiffers, &b.PaidOutside)
	return b, err
}
