package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/GainForest/hyperindex/internal/database"
	"github.com/GainForest/hyperindex/internal/oauth"
)

// OAuthAuthorizationCodesRepository handles OAuth authorization code persistence.
type OAuthAuthorizationCodesRepository struct {
	db database.Executor
}

// NewOAuthAuthorizationCodesRepository creates a new OAuth authorization codes repository.
func NewOAuthAuthorizationCodesRepository(db database.Executor) *OAuthAuthorizationCodesRepository {
	return &OAuthAuthorizationCodesRepository{db: db}
}

// Insert creates a new OAuth authorization code.
func (r *OAuthAuthorizationCodesRepository) Insert(ctx context.Context, code *oauth.AuthorizationCode) error {
	sqlStr := fmt.Sprintf(`INSERT INTO oauth_authorization_code (
		code, client_id, user_id, session_id, session_iteration, redirect_uri,
		scope, code_challenge, code_challenge_method, nonce, created_at, expires_at, used
	) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)`,
		r.db.Placeholder(1), r.db.Placeholder(2), r.db.Placeholder(3), r.db.Placeholder(4),
		r.db.Placeholder(5), r.db.Placeholder(6), r.db.Placeholder(7), r.db.Placeholder(8),
		r.db.Placeholder(9), r.db.Placeholder(10), r.db.Placeholder(11), r.db.Placeholder(12),
		r.db.Placeholder(13))

	used := 0
	if code.Used {
		used = 1
	}

	params := []database.Value{
		database.Text(code.Code),
		database.Text(code.ClientID),
		database.Text(code.UserID),
		database.NullableText(code.SessionID),
		database.NullableInt(code.SessionIteration),
		database.Text(code.RedirectURI),
		database.NullableText(code.Scope),
		database.NullableText(code.CodeChallenge),
		database.NullableText(code.CodeChallengeMethod),
		database.NullableText(code.Nonce),
		database.Int(code.CreatedAt),
		database.Int(code.ExpiresAt),
		database.Int(int64(used)),
	}

	_, err := r.db.Exec(ctx, sqlStr, params)
	return err
}

// Get retrieves an OAuth authorization code by code string.
func (r *OAuthAuthorizationCodesRepository) Get(ctx context.Context, codeStr string) (*oauth.AuthorizationCode, error) {
	sqlStr := fmt.Sprintf(`SELECT code, client_id, user_id, session_id, session_iteration, redirect_uri,
		scope, code_challenge, code_challenge_method, nonce, created_at, expires_at, used
	FROM oauth_authorization_code WHERE code = %s`, r.db.Placeholder(1))

	var (
		code                oauth.AuthorizationCode
		sessionID           sql.NullString
		sessionIteration    sql.NullInt64
		scope               sql.NullString
		codeChallenge       sql.NullString
		codeChallengeMethod sql.NullString
		nonce               sql.NullString
		used                int
	)

	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(codeStr)},
		&code.Code, &code.ClientID, &code.UserID, &sessionID, &sessionIteration, &code.RedirectURI,
		&scope, &codeChallenge, &codeChallengeMethod, &nonce, &code.CreatedAt, &code.ExpiresAt, &used)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if sessionID.Valid {
		code.SessionID = &sessionID.String
	}
	if sessionIteration.Valid {
		code.SessionIteration = &sessionIteration.Int64
	}
	if scope.Valid {
		code.Scope = &scope.String
	}
	if codeChallenge.Valid {
		code.CodeChallenge = &codeChallenge.String
	}
	if codeChallengeMethod.Valid {
		code.CodeChallengeMethod = &codeChallengeMethod.String
	}
	if nonce.Valid {
		code.Nonce = &nonce.String
	}
	code.Used = used == 1

	return &code, nil
}

// MarkUsed marks an authorization code as used.
func (r *OAuthAuthorizationCodesRepository) MarkUsed(ctx context.Context, codeStr string) error {
	sqlStr := fmt.Sprintf("UPDATE oauth_authorization_code SET used = 1 WHERE code = %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(codeStr)})
	return err
}

// Delete removes an authorization code.
func (r *OAuthAuthorizationCodesRepository) Delete(ctx context.Context, codeStr string) error {
	sqlStr := fmt.Sprintf("DELETE FROM oauth_authorization_code WHERE code = %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(codeStr)})
	return err
}

// DeleteExpired removes all expired authorization codes.
func (r *OAuthAuthorizationCodesRepository) DeleteExpired(ctx context.Context, beforeTimestamp int64) error {
	sqlStr := fmt.Sprintf("DELETE FROM oauth_authorization_code WHERE expires_at < %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Int(beforeTimestamp)})
	return err
}
