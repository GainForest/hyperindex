package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/GainForest/hyperindex/internal/database"
	"github.com/GainForest/hyperindex/internal/oauth"
)

// OAuthAuthRequestsRepository handles OAuth authorization request persistence.
type OAuthAuthRequestsRepository struct {
	db database.Executor
}

// NewOAuthAuthRequestsRepository creates a new OAuth auth requests repository.
func NewOAuthAuthRequestsRepository(db database.Executor) *OAuthAuthRequestsRepository {
	return &OAuthAuthRequestsRepository{db: db}
}

// Insert creates a new OAuth authorization request.
func (r *OAuthAuthRequestsRepository) Insert(ctx context.Context, req *oauth.AuthRequest) error {
	sqlStr := fmt.Sprintf(`INSERT INTO oauth_auth_request (
		session_id, client_id, redirect_uri, scope, state, code_challenge,
		code_challenge_method, response_type, nonce, login_hint, created_at, expires_at
	) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)`,
		r.db.Placeholder(1), r.db.Placeholder(2), r.db.Placeholder(3), r.db.Placeholder(4),
		r.db.Placeholder(5), r.db.Placeholder(6), r.db.Placeholder(7), r.db.Placeholder(8),
		r.db.Placeholder(9), r.db.Placeholder(10), r.db.Placeholder(11), r.db.Placeholder(12))

	params := []database.Value{
		database.Text(req.SessionID),
		database.Text(req.ClientID),
		database.Text(req.RedirectURI),
		database.NullableText(req.Scope),
		database.NullableText(req.State),
		database.NullableText(req.CodeChallenge),
		database.NullableText(req.CodeChallengeMethod),
		database.Text(req.ResponseType),
		database.NullableText(req.Nonce),
		database.NullableText(req.LoginHint),
		database.Int(req.CreatedAt),
		database.Int(req.ExpiresAt),
	}

	_, err := r.db.Exec(ctx, sqlStr, params)
	return err
}

// Get retrieves an OAuth authorization request by session ID.
func (r *OAuthAuthRequestsRepository) Get(ctx context.Context, sessionID string) (*oauth.AuthRequest, error) {
	sqlStr := fmt.Sprintf(`SELECT session_id, client_id, redirect_uri, scope, state, code_challenge,
		code_challenge_method, response_type, nonce, login_hint, created_at, expires_at
	FROM oauth_auth_request WHERE session_id = %s`, r.db.Placeholder(1))

	var (
		req                 oauth.AuthRequest
		scope               sql.NullString
		state               sql.NullString
		codeChallenge       sql.NullString
		codeChallengeMethod sql.NullString
		nonce               sql.NullString
		loginHint           sql.NullString
	)

	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(sessionID)},
		&req.SessionID, &req.ClientID, &req.RedirectURI, &scope, &state, &codeChallenge,
		&codeChallengeMethod, &req.ResponseType, &nonce, &loginHint, &req.CreatedAt, &req.ExpiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if scope.Valid {
		req.Scope = &scope.String
	}
	if state.Valid {
		req.State = &state.String
	}
	if codeChallenge.Valid {
		req.CodeChallenge = &codeChallenge.String
	}
	if codeChallengeMethod.Valid {
		req.CodeChallengeMethod = &codeChallengeMethod.String
	}
	if nonce.Valid {
		req.Nonce = &nonce.String
	}
	if loginHint.Valid {
		req.LoginHint = &loginHint.String
	}

	return &req, nil
}

// Delete removes an OAuth authorization request.
func (r *OAuthAuthRequestsRepository) Delete(ctx context.Context, sessionID string) error {
	sqlStr := fmt.Sprintf("DELETE FROM oauth_auth_request WHERE session_id = %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(sessionID)})
	return err
}

// DeleteExpired removes all expired OAuth authorization requests.
func (r *OAuthAuthRequestsRepository) DeleteExpired(ctx context.Context, beforeTimestamp int64) error {
	sqlStr := fmt.Sprintf("DELETE FROM oauth_auth_request WHERE expires_at < %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Int(beforeTimestamp)})
	return err
}

// DeleteByClientID removes all OAuth authorization requests for a client.
func (r *OAuthAuthRequestsRepository) DeleteByClientID(ctx context.Context, clientID string) error {
	sqlStr := fmt.Sprintf("DELETE FROM oauth_auth_request WHERE client_id = %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(clientID)})
	return err
}
