package schema

import (
	"fmt"

	"github.com/graphql-go/graphql"

	"github.com/GainForest/hyperindex/internal/endorsement"
	"github.com/GainForest/hyperindex/internal/graphql/resolver"
	"github.com/GainForest/hyperindex/internal/oauth"
)

var endorsementAccountType = graphql.NewObject(graphql.ObjectConfig{
	Name:        "EndorsementAccount",
	Description: "One account reached by the viewer-centric Certified endorsement closure.",
	Fields: graphql.Fields{
		"did": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "DID of the account reached through active Certified endorsement awards.",
		},
		"degree": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.Int),
			Description: "Smallest endorsement hop distance from the viewer. 1 means directly endorsed by the viewer; 2 and 3 are indirect endorsements.",
		},
		"via": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(graphql.String))),
			Description: "Degree-(degree-1) predecessor DIDs that led to this account. Empty for degree-1 accounts. The list is deduplicated, sorted, and capped per account.",
		},
	},
})

var endorsementClosureResultType = graphql.NewObject(graphql.ObjectConfig{
	Name:        "EndorsementClosureResult",
	Description: "Bounded viewer-centric Certified endorsement graph closure.",
	Fields: graphql.Fields{
		"accounts": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(endorsementAccountType))),
			Description: "Accounts reachable from the viewer through active Certified endorsement awards, sorted by degree then DID.",
		},
		"truncated": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.Boolean),
			Description: "True when the server-side account cap was reached and the in-flight BFS ring was trimmed.",
		},
	},
})

func (b *Builder) createEndorsementClosureResolver() graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		viewer, _ := p.Args["viewer"].(string)
		if !oauth.IsValidDID(viewer) {
			return nil, fmt.Errorf("viewer %q is not a valid DID; use a did:plc: or did:web: identifier", viewer)
		}

		degree, _ := p.Args["degree"].(int)
		if degree < 1 || degree > endorsement.MaxDegree {
			return nil, fmt.Errorf("degree must be 1, 2, or %d; got %d", endorsement.MaxDegree, degree)
		}

		repos := resolver.GetRepositories(p.Context)
		if repos == nil || repos.Records == nil {
			return nil, fmt.Errorf("endorsement closure cannot run because record repositories are unavailable; retry after the request context is initialised")
		}

		result, err := endorsement.Compute(p.Context, repos.Records, viewer, degree, endorsement.DefaultClosureCap)
		if err != nil {
			return nil, fmt.Errorf("compute endorsement closure: %w", err)
		}

		accounts := make([]interface{}, 0, len(result.Accounts))
		for _, account := range result.Accounts {
			accounts = append(accounts, map[string]interface{}{
				"did":    account.DID,
				"degree": account.Degree,
				"via":    account.Via,
			})
		}

		return map[string]interface{}{
			"accounts":  accounts,
			"truncated": result.Truncated,
		}, nil
	}
}
