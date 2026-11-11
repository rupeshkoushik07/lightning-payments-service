package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type User struct {
	ID       int64
	Username string
	Role     string // receiver, payer or admin
}

// UpsertUser creates a user or replaces their password and role.
func (s *Store) UpsertUser(ctx context.Context, username, passwordHash, role string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO users (username, password_hash, role) VALUES ($1, $2, $3)
		ON CONFLICT (username) DO UPDATE SET password_hash = $2, role = $3`,
		username, passwordHash, role)
	return err
}

// UserForLogin returns the user and their stored password hash.
func (s *Store) UserForLogin(ctx context.Context, username string) (User, string, error) {
	var u User
	var hash string
	err := s.db.QueryRow(ctx, `SELECT id, username, role, password_hash FROM users WHERE username = $1`, username).
		Scan(&u.ID, &u.Username, &u.Role, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, "", ErrNotFound
	}
	return u, hash, err
}

func (s *Store) CreateSession(ctx context.Context, tokenHash []byte, userID int64, expires time.Time) error {
	_, err := s.db.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		tokenHash, userID, expires)
	return err
}

// SessionUser returns who a session belongs to, if it exists and has not expired.
func (s *Store) SessionUser(ctx context.Context, tokenHash []byte) (User, error) {
	var u User
	err := s.db.QueryRow(ctx, `
		SELECT u.id, u.username, u.role
		FROM   sessions s JOIN users u ON u.id = s.user_id
		WHERE  s.token_hash = $1 AND s.expires_at > now()`, tokenHash).
		Scan(&u.ID, &u.Username, &u.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash []byte) error {
	_, err := s.db.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= now()`)
	return err
}

// ListPayments returns the latest payments, newest first.
func (s *Store) ListPayments(ctx context.Context, limit int) ([]PaymentView, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, payment_hash, amount_sat, status, preimage, failure_reason, created_at, updated_at
		FROM   payments ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PaymentView
	for rows.Next() {
		var p PaymentView
		if err := rows.Scan(&p.ID, &p.PaymentHash, &p.AmountSat, &p.Status, &p.Preimage, &p.FailureReason, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListInvoices returns the latest invoices, newest first.
func (s *Store) ListInvoices(ctx context.Context, limit int) ([]InvoiceView, error) {
	rows, err := s.db.Query(ctx, `
		SELECT payment_hash, invoice, amount_sat, status, expires_at, paid_at
		FROM   invoices ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InvoiceView
	for rows.Next() {
		var v InvoiceView
		if err := rows.Scan(&v.PaymentHash, &v.Text, &v.AmountSat, &v.Status, &v.ExpiresAt, &v.PaidAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
