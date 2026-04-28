package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/GainForest/hyperindex/internal/database"
	"github.com/GainForest/hyperindex/internal/oauth"
)

// OAuthATPSessionsRepository handles ATP session persistence.
type OAuthATPSessionsRepository struct {
	db database.Executor
}

// NewOAuthATPSessionsRepository creates a new OAuth ATP sessions repository.
func NewOAuthATPSessionsRepository(db database.Executor) *OAuthATPSessionsRepository {
	return &OAuthATPSessionsRepository{db: db}
}

// Insert creates a new ATP session.
func (r *OAuthATPSessionsRepository) Insert(ctx context.Context, session *oauth.ATPSession) error {
	sqlStr := fmt.Sprintf(`INSERT INTO oauth_atp_session (
		session_id, iteration, did, session_created_at, atp_oauth_state,
		signing_key_jkt, dpop_key, access_token, refresh_token,
		access_token_created_at, access_token_expires_at, access_token_scopes,
		session_exchanged_at, exchange_error
	) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)`,
		r.db.Placeholder(1), r.db.Placeholder(2), r.db.Placeholder(3), r.db.Placeholder(4),
		r.db.Placeholder(5), r.db.Placeholder(6), r.db.Placeholder(7), r.db.Placeholder(8),
		r.db.Placeholder(9), r.db.Placeholder(10), r.db.Placeholder(11), r.db.Placeholder(12),
		r.db.Placeholder(13), r.db.Placeholder(14))

	params := []database.Value{
		database.Text(session.SessionID),
		database.Int(session.Iteration),
		database.NullableText(session.DID),
		database.Int(session.SessionCreatedAt),
		database.Text(session.ATPOAuthState),
		database.Text(session.SigningKeyJKT),
		database.Text(session.DPoPKey),
		database.NullableText(session.AccessToken),
		database.NullableText(session.RefreshToken),
		database.NullableInt(session.AccessTokenCreatedAt),
		database.NullableInt(session.AccessTokenExpiresAt),
		database.NullableText(session.AccessTokenScopes),
		database.NullableInt(session.SessionExchangedAt),
		database.NullableText(session.ExchangeError),
	}

	_, err := r.db.Exec(ctx, sqlStr, params)
	return err
}

// Get retrieves an ATP session by session ID and iteration.
func (r *OAuthATPSessionsRepository) Get(ctx context.Context, sessionID string, iteration int64) (*oauth.ATPSession, error) {
	sqlStr := fmt.Sprintf(`SELECT session_id, iteration, did, session_created_at, atp_oauth_state,
		signing_key_jkt, dpop_key, access_token, refresh_token,
		access_token_created_at, access_token_expires_at, access_token_scopes,
		session_exchanged_at, exchange_error
	FROM oauth_atp_session WHERE session_id = %s AND iteration = %s`, r.db.Placeholder(1), r.db.Placeholder(2))

	session, err := r.scanSession(ctx, sqlStr, []database.Value{database.Text(sessionID), database.Int(iteration)})
	if err != nil {
		return nil, err
	}

	return session, nil
}

// GetLatest retrieves the latest iteration of an ATP session.
func (r *OAuthATPSessionsRepository) GetLatest(ctx context.Context, sessionID string) (*oauth.ATPSession, error) {
	sqlStr := fmt.Sprintf(`SELECT session_id, iteration, did, session_created_at, atp_oauth_state,
		signing_key_jkt, dpop_key, access_token, refresh_token,
		access_token_created_at, access_token_expires_at, access_token_scopes,
		session_exchanged_at, exchange_error
	FROM oauth_atp_session WHERE session_id = %s ORDER BY iteration DESC LIMIT 1`, r.db.Placeholder(1))

	session, err := r.scanSession(ctx, sqlStr, []database.Value{database.Text(sessionID)})
	if err != nil {
		return nil, err
	}

	return session, nil
}

// GetByDID retrieves the latest ATP session for a DID.
func (r *OAuthATPSessionsRepository) GetByDID(ctx context.Context, did string) (*oauth.ATPSession, error) {
	sqlStr := fmt.Sprintf(`SELECT session_id, iteration, did, session_created_at, atp_oauth_state,
		signing_key_jkt, dpop_key, access_token, refresh_token,
		access_token_created_at, access_token_expires_at, access_token_scopes,
		session_exchanged_at, exchange_error
	FROM oauth_atp_session WHERE did = %s ORDER BY session_created_at DESC LIMIT 1`, r.db.Placeholder(1))

	session, err := r.scanSession(ctx, sqlStr, []database.Value{database.Text(did)})
	if err != nil {
		return nil, err
	}

	return session, nil
}

// GetByAccessToken retrieves an ATP session by access token.
func (r *OAuthATPSessionsRepository) GetByAccessToken(ctx context.Context, accessToken string) (*oauth.ATPSession, error) {
	sqlStr := fmt.Sprintf(`SELECT session_id, iteration, did, session_created_at, atp_oauth_state,
		signing_key_jkt, dpop_key, access_token, refresh_token,
		access_token_created_at, access_token_expires_at, access_token_scopes,
		session_exchanged_at, exchange_error
	FROM oauth_atp_session WHERE access_token = %s`, r.db.Placeholder(1))

	session, err := r.scanSession(ctx, sqlStr, []database.Value{database.Text(accessToken)})
	if err != nil {
		return nil, err
	}

	return session, nil
}

// Update updates an existing ATP session.
func (r *OAuthATPSessionsRepository) Update(ctx context.Context, session *oauth.ATPSession) error {
	sqlStr := fmt.Sprintf(`UPDATE oauth_atp_session SET
		did = %s, access_token = %s, refresh_token = %s,
		access_token_created_at = %s, access_token_expires_at = %s, access_token_scopes = %s,
		session_exchanged_at = %s, exchange_error = %s
	WHERE session_id = %s AND iteration = %s`,
		r.db.Placeholder(1), r.db.Placeholder(2), r.db.Placeholder(3),
		r.db.Placeholder(4), r.db.Placeholder(5), r.db.Placeholder(6),
		r.db.Placeholder(7), r.db.Placeholder(8), r.db.Placeholder(9), r.db.Placeholder(10))

	params := []database.Value{
		database.NullableText(session.DID),
		database.NullableText(session.AccessToken),
		database.NullableText(session.RefreshToken),
		database.NullableInt(session.AccessTokenCreatedAt),
		database.NullableInt(session.AccessTokenExpiresAt),
		database.NullableText(session.AccessTokenScopes),
		database.NullableInt(session.SessionExchangedAt),
		database.NullableText(session.ExchangeError),
		database.Text(session.SessionID),
		database.Int(session.Iteration),
	}

	_, err := r.db.Exec(ctx, sqlStr, params)
	return err
}

// Delete removes an ATP session.
func (r *OAuthATPSessionsRepository) Delete(ctx context.Context, sessionID string, iteration int64) error {
	sqlStr := fmt.Sprintf("DELETE FROM oauth_atp_session WHERE session_id = %s AND iteration = %s",
		r.db.Placeholder(1), r.db.Placeholder(2))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(sessionID), database.Int(iteration)})
	return err
}

// DeleteAllIterations removes all iterations of an ATP session.
func (r *OAuthATPSessionsRepository) DeleteAllIterations(ctx context.Context, sessionID string) error {
	sqlStr := fmt.Sprintf("DELETE FROM oauth_atp_session WHERE session_id = %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(sessionID)})
	return err
}

// Helper to scan session from query
func (r *OAuthATPSessionsRepository) scanSession(ctx context.Context, sqlStr string, params []database.Value) (*oauth.ATPSession, error) {
	var (
		session              oauth.ATPSession
		did                  sql.NullString
		accessToken          sql.NullString
		refreshToken         sql.NullString
		accessTokenCreatedAt sql.NullInt64
		accessTokenExpiresAt sql.NullInt64
		accessTokenScopes    sql.NullString
		sessionExchangedAt   sql.NullInt64
		exchangeError        sql.NullString
	)

	err := r.db.QueryRow(ctx, sqlStr, params,
		&session.SessionID, &session.Iteration, &did, &session.SessionCreatedAt, &session.ATPOAuthState,
		&session.SigningKeyJKT, &session.DPoPKey, &accessToken, &refreshToken,
		&accessTokenCreatedAt, &accessTokenExpiresAt, &accessTokenScopes,
		&sessionExchangedAt, &exchangeError)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if did.Valid {
		session.DID = &did.String
	}
	if accessToken.Valid {
		session.AccessToken = &accessToken.String
	}
	if refreshToken.Valid {
		session.RefreshToken = &refreshToken.String
	}
	if accessTokenCreatedAt.Valid {
		session.AccessTokenCreatedAt = &accessTokenCreatedAt.Int64
	}
	if accessTokenExpiresAt.Valid {
		session.AccessTokenExpiresAt = &accessTokenExpiresAt.Int64
	}
	if accessTokenScopes.Valid {
		session.AccessTokenScopes = &accessTokenScopes.String
	}
	if sessionExchangedAt.Valid {
		session.SessionExchangedAt = &sessionExchangedAt.Int64
	}
	if exchangeError.Valid {
		session.ExchangeError = &exchangeError.String
	}

	return &session, nil
}
