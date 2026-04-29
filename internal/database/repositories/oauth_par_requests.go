package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/GainForest/hyperindex/internal/database"
	"github.com/GainForest/hyperindex/internal/oauth"
)

// OAuthPARRequestsRepository handles Pushed Authorization Request persistence.
type OAuthPARRequestsRepository struct {
	db database.Executor
}

// NewOAuthPARRequestsRepository creates a new OAuth PAR requests repository.
func NewOAuthPARRequestsRepository(db database.Executor) *OAuthPARRequestsRepository {
	return &OAuthPARRequestsRepository{db: db}
}

// Insert creates a new PAR request.
func (r *OAuthPARRequestsRepository) Insert(ctx context.Context, par *oauth.PARRequest) error {
	var sqlStr string
	switch r.db.Dialect() {
	case database.PostgreSQL:
		sqlStr = fmt.Sprintf(`INSERT INTO oauth_par_request (
			request_uri, authorization_request, client_id, created_at, expires_at, subject, metadata
		) VALUES (%s, %s, %s, %s, %s, %s, %s::jsonb)`,
			r.db.Placeholder(1), r.db.Placeholder(2), r.db.Placeholder(3), r.db.Placeholder(4),
			r.db.Placeholder(5), r.db.Placeholder(6), r.db.Placeholder(7))
	default:
		sqlStr = fmt.Sprintf(`INSERT INTO oauth_par_request (
			request_uri, authorization_request, client_id, created_at, expires_at, subject, metadata
		) VALUES (%s, %s, %s, %s, %s, %s, %s)`,
			r.db.Placeholder(1), r.db.Placeholder(2), r.db.Placeholder(3), r.db.Placeholder(4),
			r.db.Placeholder(5), r.db.Placeholder(6), r.db.Placeholder(7))
	}

	params := []database.Value{
		database.Text(par.RequestURI),
		database.Text(par.AuthorizationRequest),
		database.Text(par.ClientID),
		database.Int(par.CreatedAt),
		database.Int(par.ExpiresAt),
		database.NullableText(par.Subject),
		database.Text(par.Metadata),
	}

	_, err := r.db.Exec(ctx, sqlStr, params)
	return err
}

// Get retrieves a PAR request by request URI.
func (r *OAuthPARRequestsRepository) Get(ctx context.Context, requestURI string) (*oauth.PARRequest, error) {
	var sqlStr string
	switch r.db.Dialect() {
	case database.PostgreSQL:
		sqlStr = fmt.Sprintf(`SELECT request_uri, authorization_request, client_id, created_at, expires_at, subject, metadata::text
		FROM oauth_par_request WHERE request_uri = %s`, r.db.Placeholder(1))
	default:
		sqlStr = fmt.Sprintf(`SELECT request_uri, authorization_request, client_id, created_at, expires_at, subject, metadata
		FROM oauth_par_request WHERE request_uri = %s`, r.db.Placeholder(1))
	}

	var (
		par     oauth.PARRequest
		subject sql.NullString
	)

	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(requestURI)},
		&par.RequestURI, &par.AuthorizationRequest, &par.ClientID, &par.CreatedAt, &par.ExpiresAt, &subject, &par.Metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if subject.Valid {
		par.Subject = &subject.String
	}

	return &par, nil
}

// UpdateSubject sets the subject on a PAR request (after user authenticates).
func (r *OAuthPARRequestsRepository) UpdateSubject(ctx context.Context, requestURI, subject string) error {
	sqlStr := fmt.Sprintf("UPDATE oauth_par_request SET subject = %s WHERE request_uri = %s",
		r.db.Placeholder(1), r.db.Placeholder(2))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(subject), database.Text(requestURI)})
	return err
}

// Delete removes a PAR request.
func (r *OAuthPARRequestsRepository) Delete(ctx context.Context, requestURI string) error {
	sqlStr := fmt.Sprintf("DELETE FROM oauth_par_request WHERE request_uri = %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(requestURI)})
	return err
}

// DeleteExpired removes all expired PAR requests.
func (r *OAuthPARRequestsRepository) DeleteExpired(ctx context.Context, beforeTimestamp int64) error {
	sqlStr := fmt.Sprintf("DELETE FROM oauth_par_request WHERE expires_at < %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Int(beforeTimestamp)})
	return err
}
