package schema

import (
	"context"
	"fmt"
	"strconv"

	"github.com/graphql-go/graphql"

	"github.com/GainForest/hyperindex/internal/graphclosure"
	"github.com/GainForest/hyperindex/internal/graphql/query"
	"github.com/GainForest/hyperindex/internal/graphql/resolver"
	"github.com/GainForest/hyperindex/internal/oauth"
)

var endorsementAccountType = graphql.NewObject(graphql.ObjectConfig{
	Name:        "EndorsementAccount",
	Description: "One account reached by the DID-rooted Certified endorsement closure.",
	Fields: graphql.Fields{
		"did": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "DID of the account reached through active Certified endorsement awards.",
		},
		"degree": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.Int),
			Description: "Smallest endorsement hop distance from the root DID. 1 means directly endorsed by the root DID; 2 and 3 are indirect endorsements.",
		},
		"via": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(graphql.String))),
			Description: "Degree-(degree-1) predecessor DIDs that led to this account. Empty for degree-1 accounts. The list is deduplicated, sorted, and capped per account.",
		},
	},
})

var endorsementClosureDIDFilterInput = graphql.NewInputObject(graphql.InputObjectConfig{
	Name:        "EndorsementClosureDIDFilterInput",
	Description: "Exact root DID filter for endorsementClosure. Only eq is supported because each closure is computed from one DID.",
	Fields: graphql.InputObjectConfigFieldMap{
		"eq": &graphql.InputObjectFieldConfig{
			Type:        graphql.String,
			Description: "Root DID for the endorsement closure.",
		},
	},
})

var endorsementClosureDegreeFilterInput = graphql.NewInputObject(graphql.InputObjectConfig{
	Name:        "EndorsementClosureDegreeFilterInput",
	Description: "Optional exact filter for endorsement closure hop distance. Values must be 1, 2, or 3.",
	Fields: graphql.InputObjectConfigFieldMap{
		"eq": &graphql.InputObjectFieldConfig{
			Type:        graphql.Int,
			Description: "Equal to this endorsement hop distance.",
		},
	},
})

var endorsementClosureWhereInput = graphql.NewInputObject(graphql.InputObjectConfig{
	Name:        "EndorsementClosureWhereInput",
	Description: "Filters for the Certified endorsement closure. did.eq is required and selects the root DID. Optional degree.eq selects one returned hop distance from 1 through 3.",
	Fields: graphql.InputObjectConfigFieldMap{
		"did": &graphql.InputObjectFieldConfig{
			Type:        graphql.NewNonNull(endorsementClosureDIDFilterInput),
			Description: "Root DID filter. endorsementClosure exposes only did.eq because each closure is computed from one DID.",
		},
		"degree": &graphql.InputObjectFieldConfig{
			Type:        endorsementClosureDegreeFilterInput,
			Description: "Optional returned-degree filter. Supports eq with values from 1 through 3.",
		},
	},
})

var endorsementClosureEdgeType = graphql.NewObject(graphql.ObjectConfig{
	Name:        "EndorsementAccountEdge",
	Description: "An edge in the endorsement closure connection.",
	Fields: graphql.Fields{
		"cursor": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "Opaque cursor for this endorsement account.",
		},
		"node": &graphql.Field{
			Type:        graphql.NewNonNull(endorsementAccountType),
			Description: "The endorsement account at the end of the edge.",
		},
	},
})

var endorsementClosureConnectionType = graphql.NewObject(graphql.ObjectConfig{
	Name:        "EndorsementClosureConnection",
	Description: "A paginated, bounded Certified endorsement graph closure rooted at one DID.",
	Fields: graphql.Fields{
		"edges": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(endorsementClosureEdgeType))),
			Description: "Closure accounts in fixed degree-ascending, DID-ascending order.",
		},
		"pageInfo": &graphql.Field{
			Type:        graphql.NewNonNull(query.PageInfoType),
			Description: "Pagination information for the closure connection.",
		},
		"totalCount": &graphql.Field{
			Type:        graphql.Int,
			Description: "Total number of accounts in the filtered closure before pagination.",
		},
		"truncated": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.Boolean),
			Description: "True when the server-side account cap was reached and the in-flight BFS ring was trimmed.",
		},
	},
})

type endorsementClosureWhere struct {
	RootDID   string
	MinDegree int
	MaxDegree int
}

type endorsementAdjacencySource interface {
	EndorsementAdjacencyForLimit(ctx context.Context, issuers []string, limit int) (map[string][]string, bool, error)
}

type endorsementClosureAdjacency struct {
	source endorsementAdjacencySource
}

func (a endorsementClosureAdjacency) AdjacentForLimit(ctx context.Context, sources []string, limit int) (map[string][]string, bool, error) {
	return a.source.EndorsementAdjacencyForLimit(ctx, sources, limit)
}

func (b *Builder) createEndorsementClosureResolver() graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		where, err := parseEndorsementClosureWhere(p.Args["where"])
		if err != nil {
			return nil, err
		}

		repos := resolver.GetRepositories(p.Context)
		if repos == nil || repos.Records == nil {
			return nil, fmt.Errorf("endorsement closure cannot run because record repositories are unavailable; retry after the request context is initialised")
		}

		adjacency := endorsementClosureAdjacency{source: repos.Records}
		result, err := graphclosure.Compute(p.Context, adjacency, where.RootDID, where.MaxDegree, graphclosure.DefaultClosureCap)
		if err != nil {
			return nil, fmt.Errorf("compute endorsement closure: %w", err)
		}

		accounts := filterEndorsementClosureAccounts(result.Accounts, where)
		return paginateEndorsementClosureAccounts(accounts, result.Truncated, p.Args)
	}
}

func parseEndorsementClosureWhere(raw interface{}) (endorsementClosureWhere, error) {
	whereMap, ok := raw.(map[string]interface{})
	if !ok || whereMap == nil {
		return endorsementClosureWhere{}, fmt.Errorf("endorsementClosure requires where.did.eq with a valid did:plc: or did:web: identifier")
	}

	didFilter, ok := whereMap["did"].(map[string]interface{})
	if !ok || didFilter == nil {
		return endorsementClosureWhere{}, fmt.Errorf("endorsementClosure requires where.did.eq with a valid did:plc: or did:web: identifier")
	}
	if _, hasIn := didFilter["in"]; hasIn {
		return endorsementClosureWhere{}, fmt.Errorf("endorsementClosure supports one root DID per request; use where.did.eq instead of where.did.in")
	}
	rootDID, _ := didFilter["eq"].(string)
	if !oauth.IsValidDID(rootDID) {
		return endorsementClosureWhere{}, fmt.Errorf("where.did.eq %q is not a valid DID; use a did:plc: or did:web: identifier", rootDID)
	}

	where := endorsementClosureWhere{RootDID: rootDID, MinDegree: 1, MaxDegree: graphclosure.MaxDegree}
	if rawDegree, exists := whereMap["degree"]; exists && rawDegree != nil {
		degreeFilter, ok := rawDegree.(map[string]interface{})
		if !ok {
			return endorsementClosureWhere{}, fmt.Errorf("where.degree must be an object with supported operator eq")
		}
		if err := applyEndorsementClosureDegreeFilter(&where, degreeFilter); err != nil {
			return endorsementClosureWhere{}, err
		}
	}
	if where.MinDegree > where.MaxDegree {
		return endorsementClosureWhere{}, fmt.Errorf("where.degree selects no valid endorsement degrees; use values from 1 through %d", graphclosure.MaxDegree)
	}
	return where, nil
}

func applyEndorsementClosureDegreeFilter(where *endorsementClosureWhere, degreeFilter map[string]interface{}) error {
	for op := range degreeFilter {
		if op != "eq" {
			return fmt.Errorf("where.degree.%s is not supported by endorsementClosure; use where.degree.eq", op)
		}
	}
	value, exists := degreeFilter["eq"]
	if !exists || value == nil {
		return nil
	}
	degree, err := endorsementClosureDegreeValue("eq", value)
	if err != nil {
		return err
	}
	where.MinDegree = degree
	where.MaxDegree = degree
	return nil
}

func endorsementClosureDegreeValue(op string, raw interface{}) (int, error) {
	degree, ok := raw.(int)
	if !ok {
		return 0, fmt.Errorf("where.degree.%s must be an integer from 1 through %d", op, graphclosure.MaxDegree)
	}
	if degree < 1 || degree > graphclosure.MaxDegree {
		return 0, fmt.Errorf("where.degree.%s must be between 1 and %d, got %d", op, graphclosure.MaxDegree, degree)
	}
	return degree, nil
}

func filterEndorsementClosureAccounts(accounts []graphclosure.Account, where endorsementClosureWhere) []graphclosure.Account {
	filtered := make([]graphclosure.Account, 0, len(accounts))
	for _, account := range accounts {
		if account.Degree < where.MinDegree || account.Degree > where.MaxDegree {
			continue
		}
		filtered = append(filtered, account)
	}
	return filtered
}

func paginateEndorsementClosureAccounts(accounts []graphclosure.Account, truncated bool, args map[string]interface{}) (map[string]interface{}, error) {
	pageSize, _ := args["first"].(int)
	pageSize = query.ClampPageSize(pageSize)

	start := 0
	if after, ok := args["after"].(string); ok && after != "" {
		cursorDegree, cursorDID, err := decodeEndorsementClosureCursor(after)
		if err != nil {
			return nil, fmt.Errorf("invalid endorsementClosure cursor: %w", err)
		}
		start = endorsementClosureStartOffset(accounts, cursorDegree, cursorDID)
	}

	end := start + pageSize
	if end > len(accounts) {
		end = len(accounts)
	}
	page := accounts[start:end]

	edges := make([]interface{}, 0, len(page))
	var startCursor interface{}
	var endCursor interface{}
	for _, account := range page {
		cursor := encodeEndorsementClosureCursor(account)
		if startCursor == nil {
			startCursor = cursor
		}
		endCursor = cursor
		edges = append(edges, map[string]interface{}{
			"cursor": cursor,
			"node": map[string]interface{}{
				"did":    account.DID,
				"degree": account.Degree,
				"via":    account.Via,
			},
		})
	}

	return map[string]interface{}{
		"edges": edges,
		"pageInfo": map[string]interface{}{
			"hasNextPage":     end < len(accounts),
			"hasPreviousPage": start > 0,
			"startCursor":     startCursor,
			"endCursor":       endCursor,
		},
		"totalCount": len(accounts),
		"truncated":  truncated,
	}, nil
}

func encodeEndorsementClosureCursor(account graphclosure.Account) string {
	return encodeCursorValues(strconv.Itoa(account.Degree), account.DID)
}

func decodeEndorsementClosureCursor(cursor string) (int, string, error) {
	parts, err := decodeCursorValues(cursor)
	if err != nil {
		return 0, "", err
	}
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("expected degree and DID components")
	}
	degree, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", fmt.Errorf("degree component must be an integer")
	}
	if degree < 1 || degree > graphclosure.MaxDegree {
		return 0, "", fmt.Errorf("degree component must be between 1 and %d", graphclosure.MaxDegree)
	}
	if !oauth.IsValidDID(parts[1]) {
		return 0, "", fmt.Errorf("DID component must be a did:plc: or did:web: identifier")
	}
	return degree, parts[1], nil
}

func endorsementClosureStartOffset(accounts []graphclosure.Account, cursorDegree int, cursorDID string) int {
	for i, account := range accounts {
		if account.Degree > cursorDegree || (account.Degree == cursorDegree && account.DID > cursorDID) {
			return i
		}
	}
	return len(accounts)
}
