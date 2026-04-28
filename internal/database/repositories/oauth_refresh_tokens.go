package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/GainForest/hyperindex/internal/database"
	"github.com/GainForest/hyperindex/internal/oauth"
)

// OAuthRefreshTokensRepository handles OAuth refresh token persistence.
type OAuthRefreshTokensRepository struct {
	db database.Executor
}

// NewOAuthRefreshTokensRepository creates a new OAuth refresh tokens repository.
func NewOAuthRefreshTokensRepository(db database.Executor) *OAuthRefreshTokensRepository {
	return &OAuthRefreshTokensRepository{db: db}
}

// Insert creates a new OAuth refresh token.
func (r *OAuthRefreshTokensRepository) Insert(ctx context.Context, token *oauth.RefreshToken) error {
	sqlStr := fmt.Sprintf(`INSERT INTO oauth_refresh_token (
		token, access_token, client_id, user_id, session_id,
		session_iteration, scope, created_at, expires_at, revoked
	) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s)`,
		r.db.Placeholder(1), r.db.Placeholder(2), r.db.Placeholder(3), r.db.Placeholder(4),
		r.db.Placeholder(5), r.db.Placeholder(6), r.db.Placeholder(7), r.db.Placeholder(8),
		r.db.Placeholder(9), r.db.Placeholder(10))

	revoked := 0
	if token.Revoked {
		revoked = 1
	}

	params := []database.Value{
		database.Text(token.Token),
		database.Text(token.AccessToken),
		database.Text(token.ClientID),
		database.Text(token.UserID),
		database.NullableText(token.SessionID),
		database.NullableInt(token.SessionIteration),
		database.NullableText(token.Scope),
		database.Int(token.CreatedAt),
		database.NullableInt(token.ExpiresAt),
		database.Int(int64(revoked)),
	}

	_, err := r.db.Exec(ctx, sqlStr, params)
	return err
}

// Get retrieves an OAuth refresh token by token string.
func (r *OAuthRefreshTokensRepository) Get(ctx context.Context, tokenStr string) (*oauth.RefreshToken, error) {
	sqlStr := fmt.Sprintf(`SELECT token, access_token, client_id, user_id, session_id,
		session_iteration, scope, created_at, expires_at, revoked
	FROM oauth_refresh_token WHERE token = %s`, r.db.Placeholder(1))

	var (
		token            oauth.RefreshToken
		sessionID        sql.NullString
		sessionIteration sql.NullInt64
		scope            sql.NullString
		expiresAt        sql.NullInt64
		revoked          int
	)

	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(tokenStr)},
		&token.Token, &token.AccessToken, &token.ClientID, &token.UserID, &sessionID,
		&sessionIteration, &scope, &token.CreatedAt, &expiresAt, &revoked)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if sessionID.Valid {
		token.SessionID = &sessionID.String
	}
	if sessionIteration.Valid {
		token.SessionIteration = &sessionIteration.Int64
	}
	if scope.Valid {
		token.Scope = &scope.String
	}
	if expiresAt.Valid {
		token.ExpiresAt = &expiresAt.Int64
	}
	token.Revoked = revoked == 1

	return &token, nil
}

// GetByAccessToken retrieves a refresh token by its associated access token.
func (r *OAuthRefreshTokensRepository) GetByAccessToken(ctx context.Context, accessToken string) (*oauth.RefreshToken, error) {
	sqlStr := fmt.Sprintf(`SELECT token, access_token, client_id, user_id, session_id,
		session_iteration, scope, created_at, expires_at, revoked
	FROM oauth_refresh_token WHERE access_token = %s`, r.db.Placeholder(1))

	var (
		token            oauth.RefreshToken
		sessionID        sql.NullString
		sessionIteration sql.NullInt64
		scope            sql.NullString
		expiresAt        sql.NullInt64
		revoked          int
	)

	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(accessToken)},
		&token.Token, &token.AccessToken, &token.ClientID, &token.UserID, &sessionID,
		&sessionIteration, &scope, &token.CreatedAt, &expiresAt, &revoked)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if sessionID.Valid {
		token.SessionID = &sessionID.String
	}
	if sessionIteration.Valid {
		token.SessionIteration = &sessionIteration.Int64
	}
	if scope.Valid {
		token.Scope = &scope.String
	}
	if expiresAt.Valid {
		token.ExpiresAt = &expiresAt.Int64
	}
	token.Revoked = revoked == 1

	return &token, nil
}

// GetByUserID retrieves all refresh tokens for a user.
func (r *OAuthRefreshTokensRepository) GetByUserID(ctx context.Context, userID string) ([]*oauth.RefreshToken, error) {
	sqlStr := fmt.Sprintf(`SELECT token, access_token, client_id, user_id, session_id,
		session_iteration, scope, created_at, expires_at, revoked
	FROM oauth_refresh_token WHERE user_id = %s ORDER BY created_at DESC`, r.db.Placeholder(1))

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []*oauth.RefreshToken
	for rows.Next() {
		token, err := scanRefreshToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}

	return tokens, rows.Err()
}

// Revoke marks a refresh token as revoked.
func (r *OAuthRefreshTokensRepository) Revoke(ctx context.Context, tokenStr string) error {
	sqlStr := fmt.Sprintf("UPDATE oauth_refresh_token SET revoked = 1 WHERE token = %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(tokenStr)})
	return err
}

// RevokeByUserID revokes all refresh tokens for a user.
func (r *OAuthRefreshTokensRepository) RevokeByUserID(ctx context.Context, userID string) error {
	sqlStr := fmt.Sprintf("UPDATE oauth_refresh_token SET revoked = 1 WHERE user_id = %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(userID)})
	return err
}

// Delete removes a refresh token.
func (r *OAuthRefreshTokensRepository) Delete(ctx context.Context, tokenStr string) error {
	sqlStr := fmt.Sprintf("DELETE FROM oauth_refresh_token WHERE token = %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(tokenStr)})
	return err
}

// DeleteExpired removes all expired refresh tokens.
func (r *OAuthRefreshTokensRepository) DeleteExpired(ctx context.Context, beforeTimestamp int64) error {
	sqlStr := fmt.Sprintf("DELETE FROM oauth_refresh_token WHERE expires_at IS NOT NULL AND expires_at < %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Int(beforeTimestamp)})
	return err
}

// Helper function to scan refresh token rows
func scanRefreshToken(rows *sql.Rows) (*oauth.RefreshToken, error) {
	var (
		token            oauth.RefreshToken
		sessionID        sql.NullString
		sessionIteration sql.NullInt64
		scope            sql.NullString
		expiresAt        sql.NullInt64
		revoked          int
	)

	err := rows.Scan(
		&token.Token, &token.AccessToken, &token.ClientID, &token.UserID, &sessionID,
		&sessionIteration, &scope, &token.CreatedAt, &expiresAt, &revoked)
	if err != nil {
		return nil, err
	}

	if sessionID.Valid {
		token.SessionID = &sessionID.String
	}
	if sessionIteration.Valid {
		token.SessionIteration = &sessionIteration.Int64
	}
	if scope.Valid {
		token.Scope = &scope.String
	}
	if expiresAt.Valid {
		token.ExpiresAt = &expiresAt.Int64
	}
	token.Revoked = revoked == 1

	return &token, nil
}
