package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/GainForest/hyperindex/internal/database"
	"github.com/GainForest/hyperindex/internal/oauth"
)

// OAuthAccessTokensRepository handles OAuth access token persistence.
type OAuthAccessTokensRepository struct {
	db database.Executor
}

// NewOAuthAccessTokensRepository creates a new OAuth access tokens repository.
func NewOAuthAccessTokensRepository(db database.Executor) *OAuthAccessTokensRepository {
	return &OAuthAccessTokensRepository{db: db}
}

// Insert creates a new OAuth access token.
func (r *OAuthAccessTokensRepository) Insert(ctx context.Context, token *oauth.AccessToken) error {
	sqlStr := fmt.Sprintf(`INSERT INTO oauth_access_token (
		token, token_type, client_id, user_id, session_id,
		session_iteration, scope, created_at, expires_at, revoked, dpop_jkt
	) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)`,
		r.db.Placeholder(1), r.db.Placeholder(2), r.db.Placeholder(3), r.db.Placeholder(4),
		r.db.Placeholder(5), r.db.Placeholder(6), r.db.Placeholder(7), r.db.Placeholder(8),
		r.db.Placeholder(9), r.db.Placeholder(10), r.db.Placeholder(11))

	revoked := 0
	if token.Revoked {
		revoked = 1
	}

	params := []database.Value{
		database.Text(token.Token),
		database.Text(string(token.TokenType)),
		database.Text(token.ClientID),
		database.NullableText(token.UserID),
		database.NullableText(token.SessionID),
		database.NullableInt(token.SessionIteration),
		database.NullableText(token.Scope),
		database.Int(token.CreatedAt),
		database.Int(token.ExpiresAt),
		database.Int(int64(revoked)),
		database.NullableText(token.DPoPJKT),
	}

	_, err := r.db.Exec(ctx, sqlStr, params)
	return err
}

// Get retrieves an OAuth access token by token string.
func (r *OAuthAccessTokensRepository) Get(ctx context.Context, tokenStr string) (*oauth.AccessToken, error) {
	sqlStr := fmt.Sprintf(`SELECT token, token_type, client_id, user_id, session_id,
		session_iteration, scope, created_at, expires_at, revoked, dpop_jkt
	FROM oauth_access_token WHERE token = %s`, r.db.Placeholder(1))

	var (
		token            oauth.AccessToken
		tokenType        string
		userID           sql.NullString
		sessionID        sql.NullString
		sessionIteration sql.NullInt64
		scope            sql.NullString
		revoked          int
		dpopJKT          sql.NullString
	)

	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(tokenStr)},
		&token.Token, &tokenType, &token.ClientID, &userID, &sessionID,
		&sessionIteration, &scope, &token.CreatedAt, &token.ExpiresAt, &revoked, &dpopJKT)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	token.TokenType = oauth.TokenType(tokenType)
	if userID.Valid {
		token.UserID = &userID.String
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
	token.Revoked = revoked == 1
	if dpopJKT.Valid {
		token.DPoPJKT = &dpopJKT.String
	}

	return &token, nil
}

// GetByUserID retrieves all access tokens for a user.
func (r *OAuthAccessTokensRepository) GetByUserID(ctx context.Context, userID string) ([]*oauth.AccessToken, error) {
	sqlStr := fmt.Sprintf(`SELECT token, token_type, client_id, user_id, session_id,
		session_iteration, scope, created_at, expires_at, revoked, dpop_jkt
	FROM oauth_access_token WHERE user_id = %s ORDER BY created_at DESC`, r.db.Placeholder(1))

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []*oauth.AccessToken
	for rows.Next() {
		token, err := scanAccessToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}

	return tokens, rows.Err()
}

// GetByDPoPJKT retrieves all access tokens for a DPoP JKT.
func (r *OAuthAccessTokensRepository) GetByDPoPJKT(ctx context.Context, jkt string) ([]*oauth.AccessToken, error) {
	sqlStr := fmt.Sprintf(`SELECT token, token_type, client_id, user_id, session_id,
		session_iteration, scope, created_at, expires_at, revoked, dpop_jkt
	FROM oauth_access_token WHERE dpop_jkt = %s ORDER BY created_at DESC`, r.db.Placeholder(1))

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, jkt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []*oauth.AccessToken
	for rows.Next() {
		token, err := scanAccessToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}

	return tokens, rows.Err()
}

// Revoke marks an access token as revoked.
func (r *OAuthAccessTokensRepository) Revoke(ctx context.Context, tokenStr string) error {
	sqlStr := fmt.Sprintf("UPDATE oauth_access_token SET revoked = 1 WHERE token = %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(tokenStr)})
	return err
}

// RevokeByUserID revokes all access tokens for a user.
func (r *OAuthAccessTokensRepository) RevokeByUserID(ctx context.Context, userID string) error {
	sqlStr := fmt.Sprintf("UPDATE oauth_access_token SET revoked = 1 WHERE user_id = %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(userID)})
	return err
}

// Delete removes an access token.
func (r *OAuthAccessTokensRepository) Delete(ctx context.Context, tokenStr string) error {
	sqlStr := fmt.Sprintf("DELETE FROM oauth_access_token WHERE token = %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(tokenStr)})
	return err
}

// DeleteExpired removes all expired access tokens.
func (r *OAuthAccessTokensRepository) DeleteExpired(ctx context.Context, beforeTimestamp int64) error {
	sqlStr := fmt.Sprintf("DELETE FROM oauth_access_token WHERE expires_at < %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Int(beforeTimestamp)})
	return err
}

// Helper function to scan access token rows
func scanAccessToken(rows *sql.Rows) (*oauth.AccessToken, error) {
	var (
		token            oauth.AccessToken
		tokenType        string
		userID           sql.NullString
		sessionID        sql.NullString
		sessionIteration sql.NullInt64
		scope            sql.NullString
		revoked          int
		dpopJKT          sql.NullString
	)

	err := rows.Scan(
		&token.Token, &tokenType, &token.ClientID, &userID, &sessionID,
		&sessionIteration, &scope, &token.CreatedAt, &token.ExpiresAt, &revoked, &dpopJKT)
	if err != nil {
		return nil, err
	}

	token.TokenType = oauth.TokenType(tokenType)
	if userID.Valid {
		token.UserID = &userID.String
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
	token.Revoked = revoked == 1
	if dpopJKT.Valid {
		token.DPoPJKT = &dpopJKT.String
	}

	return &token, nil
}
