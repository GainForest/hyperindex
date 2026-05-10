package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/GainForest/hyperindex/internal/database"
	"github.com/GainForest/hyperindex/internal/oauth"
)

// OAuthDPoPNoncesRepository handles DPoP nonce persistence.
type OAuthDPoPNoncesRepository struct {
	db database.Executor
}

// NewOAuthDPoPNoncesRepository creates a new OAuth DPoP nonces repository.
func NewOAuthDPoPNoncesRepository(db database.Executor) *OAuthDPoPNoncesRepository {
	return &OAuthDPoPNoncesRepository{db: db}
}

// Insert creates a new DPoP nonce.
func (r *OAuthDPoPNoncesRepository) Insert(ctx context.Context, nonce *oauth.DPoPNonce) error {
	sqlStr := fmt.Sprintf(`INSERT INTO oauth_dpop_nonce (nonce, expires_at) VALUES (%s, %s)`,
		r.db.Placeholder(1), r.db.Placeholder(2))

	params := []database.Value{
		database.Text(nonce.Nonce),
		database.Int(nonce.ExpiresAt),
	}

	_, err := r.db.Exec(ctx, sqlStr, params)
	return err
}

// Get retrieves a DPoP nonce by nonce string.
func (r *OAuthDPoPNoncesRepository) Get(ctx context.Context, nonceStr string) (*oauth.DPoPNonce, error) {
	sqlStr := fmt.Sprintf(`SELECT nonce, expires_at FROM oauth_dpop_nonce WHERE nonce = %s`, r.db.Placeholder(1))

	var nonce oauth.DPoPNonce

	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(nonceStr)},
		&nonce.Nonce, &nonce.ExpiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &nonce, nil
}

// Exists checks if a nonce exists and is not expired.
func (r *OAuthDPoPNoncesRepository) Exists(ctx context.Context, nonceStr string, currentTimestamp int64) (bool, error) {
	sqlStr := fmt.Sprintf(`SELECT 1 FROM oauth_dpop_nonce WHERE nonce = %s AND expires_at > %s`,
		r.db.Placeholder(1), r.db.Placeholder(2))

	var exists int
	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(nonceStr), database.Int(currentTimestamp)}, &exists)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// Delete removes a DPoP nonce.
func (r *OAuthDPoPNoncesRepository) Delete(ctx context.Context, nonceStr string) error {
	sqlStr := fmt.Sprintf("DELETE FROM oauth_dpop_nonce WHERE nonce = %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(nonceStr)})
	return err
}

// DeleteExpired removes all expired DPoP nonces.
func (r *OAuthDPoPNoncesRepository) DeleteExpired(ctx context.Context, beforeTimestamp int64) error {
	sqlStr := fmt.Sprintf("DELETE FROM oauth_dpop_nonce WHERE expires_at < %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Int(beforeTimestamp)})
	return err
}
