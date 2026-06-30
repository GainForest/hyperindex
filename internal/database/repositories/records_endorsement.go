package repositories

import (
	"context"
	"fmt"

	"github.com/GainForest/hyperindex/internal/database"
)

const (
	certifiedBadgeAwardCollection      = "app.certified.badge.award"
	certifiedBadgeDefinitionCollection = "app.certified.badge.definition"
	certifiedBadgeResponseCollection   = "app.certified.badge.response"
	certifiedEndorsementBadgeType      = "endorsement"
)

// EndorsementAdjacencyFor returns active Certified endorsement edges grouped by
// issuer DID. An active edge is an app.certified.badge.award whose referenced
// badge definition has badgeType "endorsement", whose subject is an account DID,
// whose issuer is allowed to issue that badge, whose issuer is not the subject,
// and whose subject has not rejected the award.
func (r *RecordsRepository) EndorsementAdjacencyFor(ctx context.Context, issuers []string) (map[string][]string, error) {
	out, _, err := r.endorsementAdjacencyFor(ctx, issuers, endorsementAdjacencyQueryOptions{})
	return out, err
}

// EndorsementAdjacencyForLimit returns up to limit active Certified endorsement
// edges grouped by issuer DID. The second return value is true when more edges
// matched than were returned, allowing callers to mark graph traversals as
// truncated before materializing an unbounded result set.
func (r *RecordsRepository) EndorsementAdjacencyForLimit(ctx context.Context, issuers []string, limit int) (map[string][]string, bool, error) {
	if limit <= 0 {
		return nil, false, fmt.Errorf("endorsement adjacency limit must be positive, got %d", limit)
	}
	return r.endorsementAdjacencyFor(ctx, issuers, endorsementAdjacencyQueryOptions{Limit: limit})
}

type endorsementAdjacencyQueryOptions struct {
	Limit int
}

func (r *RecordsRepository) endorsementAdjacencyFor(ctx context.Context, issuers []string, options endorsementAdjacencyQueryOptions) (map[string][]string, bool, error) {
	out := make(map[string][]string, len(issuers))
	if len(issuers) == 0 {
		return out, false, nil
	}

	batchSize := len(issuers)
	if r.db.Dialect() == database.SQLite {
		maxBatchSize := SQLParamBatchSize
		if options.Limit > 0 {
			maxBatchSize--
		}
		if maxBatchSize <= 0 {
			return nil, false, fmt.Errorf("SQLite endorsement adjacency batch size must be positive")
		}
		if batchSize > maxBatchSize {
			batchSize = maxBatchSize
		}
	}

	remainingLimit := options.Limit
	truncated := false
	for start := 0; start < len(issuers); start += batchSize {
		end := start + batchSize
		if end > len(issuers) {
			end = len(issuers)
		}

		batchOptions := options
		if remainingLimit > 0 {
			batchOptions.Limit = remainingLimit
		}
		batchAdjacency, batchTruncated, err := r.endorsementAdjacencyBatch(ctx, issuers[start:end], batchOptions)
		if err != nil {
			return nil, false, err
		}
		for issuer, subjects := range batchAdjacency {
			out[issuer] = append(out[issuer], subjects...)
			if remainingLimit > 0 {
				remainingLimit -= len(subjects)
			}
		}
		if batchTruncated {
			truncated = true
			break
		}
		if remainingLimit == 0 && options.Limit > 0 && end < len(issuers) {
			truncated = true
			break
		}
	}

	return out, truncated, nil
}

func (r *RecordsRepository) endorsementAdjacencyBatch(ctx context.Context, issuers []string, options endorsementAdjacencyQueryOptions) (map[string][]string, bool, error) {
	placeholders := r.db.Placeholders(len(issuers), 1)
	subjectDIDExpr := r.badgeAwardSubjectDIDExpr("award.json")
	badgeURIExpr := r.jsonTextPathExpr("award.json", []string{"badge", "uri"})
	definitionBadgeTypeExpr := r.jsonTextPathExpr("definition.json", []string{"badgeType"})
	definitionAllowsIssuerExpr := r.badgeDefinitionAllowsIssuerExpr("definition.json", "endorsement_awards.issuer_did")
	responseAwardURIExpr := r.jsonTextPathExpr("response.json", []string{"badgeAward", "uri"})
	responseStateExpr := r.jsonTextPathExpr("response.json", []string{"response"})
	validSubjectDIDExpr := didSQLPredicate("endorsement_awards.subject_did")

	limitClause := ""
	if options.Limit > 0 {
		limitClause = fmt.Sprintf("\n\tLIMIT %s", r.db.Placeholder(len(issuers)+1))
	}

	sqlStr := fmt.Sprintf(`WITH endorsement_awards AS (
		SELECT
			award.did AS issuer_did,
			award.uri AS award_uri,
			%s AS subject_did,
			%s AS badge_uri
		FROM record award
		WHERE award.collection = '%s'
			AND award.did IN (%s)
	)
	SELECT DISTINCT endorsement_awards.issuer_did, endorsement_awards.subject_did
	FROM endorsement_awards
	JOIN record definition
		ON definition.uri = endorsement_awards.badge_uri
		AND definition.collection = '%s'
		AND %s = '%s'
	WHERE endorsement_awards.subject_did IS NOT NULL
		AND endorsement_awards.subject_did <> ''
		AND %s
		AND %s
		AND endorsement_awards.issuer_did <> endorsement_awards.subject_did
		AND NOT EXISTS (
			SELECT 1
			FROM record response
			WHERE response.collection = '%s'
				AND response.did = endorsement_awards.subject_did
				AND %s = endorsement_awards.award_uri
				AND %s = 'rejected'
		)
	ORDER BY endorsement_awards.issuer_did, endorsement_awards.subject_did%s`,
		subjectDIDExpr,
		badgeURIExpr,
		certifiedBadgeAwardCollection,
		placeholders,
		certifiedBadgeDefinitionCollection,
		definitionBadgeTypeExpr,
		certifiedEndorsementBadgeType,
		validSubjectDIDExpr,
		definitionAllowsIssuerExpr,
		certifiedBadgeResponseCollection,
		responseAwardURIExpr,
		responseStateExpr,
		limitClause,
	)

	args := make([]any, len(issuers), len(issuers)+1)
	for i, issuer := range issuers {
		args[i] = issuer
	}
	if options.Limit > 0 {
		args = append(args, options.Limit+1)
	}

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, false, fmt.Errorf("query Certified endorsement adjacency: %w", err)
	}
	defer rows.Close()

	out := make(map[string][]string, len(issuers))
	rowCount := 0
	truncated := false
	for rows.Next() {
		var issuer, subject string
		if err := rows.Scan(&issuer, &subject); err != nil {
			return nil, false, fmt.Errorf("scan Certified endorsement adjacency row: %w", err)
		}
		rowCount++
		if options.Limit > 0 && rowCount > options.Limit {
			truncated = true
			continue
		}
		out[issuer] = append(out[issuer], subject)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate Certified endorsement adjacency rows: %w", err)
	}

	return out, truncated, nil
}

func (r *RecordsRepository) badgeAwardSubjectDIDExpr(jsonColumn string) string {
	if r.db.Dialect() == database.PostgreSQL {
		return fmt.Sprintf(`CASE
			WHEN jsonb_typeof(%[1]s->'subject') = 'object'
				AND %[1]s->'subject'->>'$type' = 'app.certified.defs#did'
				AND %[1]s->'subject'->>'did' IS NOT NULL
				THEN %[1]s->'subject'->>'did'
			ELSE NULL
		END`, jsonColumn)
	}

	subjectTypeExpr := r.sqliteJSONTypePathExpr(jsonColumn, []string{"subject"})
	subjectRefExpr := fmt.Sprintf(`CASE WHEN json_valid(%[1]s) THEN json_extract(%[1]s, '$.subject."$type"') ELSE NULL END`, jsonColumn)
	subjectDIDExpr := r.jsonTextPathExpr(jsonColumn, []string{"subject", "did"})
	return fmt.Sprintf(`CASE
		WHEN %[1]s = 'object' AND %[2]s = 'app.certified.defs#did' THEN %[3]s
		ELSE NULL
	END`, subjectTypeExpr, subjectRefExpr, subjectDIDExpr)
}

func (r *RecordsRepository) badgeDefinitionAllowsIssuerExpr(jsonColumn, issuerExpr string) string {
	if r.db.Dialect() == database.PostgreSQL {
		return fmt.Sprintf(`(
			%[1]s->'allowedIssuers' IS NULL
			OR (
				jsonb_typeof(%[1]s->'allowedIssuers') = 'array'
				AND EXISTS (
					SELECT 1
					FROM jsonb_array_elements(%[1]s->'allowedIssuers') AS allowed_issuer(value)
					WHERE allowed_issuer.value->>'did' = %[2]s
				)
			)
		)`, jsonColumn, issuerExpr)
	}

	allowedIssuersTypeExpr := r.sqliteJSONTypePathExpr(jsonColumn, []string{"allowedIssuers"})
	return fmt.Sprintf(`(
		%[1]s IS NULL
		OR (
			%[1]s = 'array'
			AND EXISTS (
				SELECT 1
				FROM json_each(%[2]s, '$.allowedIssuers') AS allowed_issuer
				WHERE json_extract(allowed_issuer.value, '$.did') = %[3]s
			)
		)
	)`, allowedIssuersTypeExpr, jsonColumn, issuerExpr)
}

func didSQLPredicate(expr string) string {
	return fmt.Sprintf("(substr(%[1]s, 1, 8) = 'did:plc:' OR substr(%[1]s, 1, 8) = 'did:web:')", expr)
}
