// Package externallabels contains GraphQL helpers for exposing locally ingested
// external ATProto labeler events.
package externallabels

import (
	"github.com/graphql-go/graphql"

	"github.com/GainForest/hyperindex/internal/database/repositories"
)

const (
	// ActiveSourceKey is the internal source-map key used to attach active external
	// labels to record nodes before GraphQL resolves the public externalLabels field.
	ActiveSourceKey = "__externalLabelsActive"

	// HistorySourceKey is the internal source-map key used to attach historical
	// external label rows when a GraphQL field requests activeOnly: false.
	HistorySourceKey = "__externalLabelsHistory"
)

// Type is the public GraphQL object returned by external label queries and
// record-level externalLabels fields. It intentionally omits ingestion/debugging
// fields such as subscription URL, event sequence, raw JSON, and signatures.
var Type = graphql.NewObject(graphql.ObjectConfig{
	Name:        "ExternalLabel",
	Description: "An external ATProto label attached to a DID or record AT-URI.",
	Fields: graphql.Fields{
		"src": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "DID of the labeler that produced this label.",
		},
		"uri": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "DID or AT-URI subject that this label applies to.",
		},
		"cid": &graphql.Field{
			Type:        graphql.String,
			Description: "Optional CID restriction for labels that apply to one record version.",
		},
		"val": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "Label value emitted by the labeler.",
		},
		"neg": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.Boolean),
			Description: "Whether this label negates an earlier label with the same source, subject, and value.",
		},
		"cts": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "Label creation timestamp from the label event.",
		},
		"exp": &graphql.Field{
			Type:        graphql.String,
			Description: "Optional label expiration timestamp.",
		},
		"ver": &graphql.Field{
			Type:        graphql.Int,
			Description: "Optional label schema version.",
		},
	},
})

// Field creates the virtual externalLabels field injected into generated record
// types and GenericRecord. The resolver reads labels pre-attached by parent
// resolvers so record lists can hydrate labels in one batch instead of one query
// per node.
func Field() *graphql.Field {
	return &graphql.Field{
		Type:        graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(Type))),
		Description: "External ATProto labels attached to this record.",
		Args:        Args(false),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return ResolveFromSource(p.Source, p.Args), nil
		},
	}
}

// Args returns the shared argument config for external label fields. Set
// includeSubjects when building the root Query.externalLabels field.
func Args(includeSubjects bool) graphql.FieldConfigArgument {
	args := graphql.FieldConfigArgument{
		"sources": &graphql.ArgumentConfig{
			Type:        graphql.NewList(graphql.NewNonNull(graphql.String)),
			Description: "Optional labeler DIDs to include.",
		},
		"values": &graphql.ArgumentConfig{
			Type:        graphql.NewList(graphql.NewNonNull(graphql.String)),
			Description: "Optional label values to include.",
		},
		"activeOnly": &graphql.ArgumentConfig{
			Type:         graphql.Boolean,
			DefaultValue: true,
			Description:  "When true, only return current, non-negated, unexpired labels.",
		},
	}
	if includeSubjects {
		args["subjects"] = &graphql.ArgumentConfig{
			Type:        graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(graphql.String))),
			Description: "DID or AT-URI subjects whose labels should be returned.",
		}
	}
	return args
}

// ResolveFromSource returns filtered label maps from a record source map. It is
// used by the virtual externalLabels field resolver after a parent resolver has
// attached active and historical repository labels under ActiveSourceKey and
// HistorySourceKey.
func ResolveFromSource(source interface{}, args map[string]interface{}) []map[string]interface{} {
	sourceMap, ok := source.(map[string]interface{})
	if !ok {
		return []map[string]interface{}{}
	}

	filter := FilterFromArgs(args)
	key := ActiveSourceKey
	if !filter.ActiveOnly {
		key = HistorySourceKey
	}

	labels, ok := sourceMap[key].([]repositories.ExternalLabel)
	if !ok || len(labels) == 0 {
		return []map[string]interface{}{}
	}

	return ToGraphQL(ApplyFilter(labels, filter))
}

// Filter describes GraphQL-level label filters that can be applied to a hydrated
// set of labels without another database query.
type Filter struct {
	Sources    []string
	Values     []string
	ActiveOnly bool
}

// FilterFromArgs converts GraphQL resolver arguments into a Filter. Missing
// activeOnly defaults to true to match the public API contract.
func FilterFromArgs(args map[string]interface{}) Filter {
	activeOnly := true
	if value, ok := args["activeOnly"].(bool); ok {
		activeOnly = value
	}

	return Filter{
		Sources:    stringListArg(args["sources"]),
		Values:     stringListArg(args["values"]),
		ActiveOnly: activeOnly,
	}
}

// ApplyFilter restricts repository label rows by GraphQL source and value
// arguments. Active-label semantics are handled by choosing the active or
// historical hydrated label set before this function is called.
func ApplyFilter(labels []repositories.ExternalLabel, filter Filter) []repositories.ExternalLabel {
	if len(labels) == 0 {
		return nil
	}

	sourceSet := stringSet(filter.Sources)
	valueSet := stringSet(filter.Values)
	filtered := make([]repositories.ExternalLabel, 0, len(labels))
	for _, label := range labels {
		if len(sourceSet) > 0 {
			if _, ok := sourceSet[label.Src]; !ok {
				continue
			}
		}
		if len(valueSet) > 0 {
			if _, ok := valueSet[label.Val]; !ok {
				continue
			}
		}
		filtered = append(filtered, label)
	}
	return filtered
}

// ToGraphQL converts repository labels into map values that match the public
// ExternalLabel GraphQL type.
func ToGraphQL(labels []repositories.ExternalLabel) []map[string]interface{} {
	if len(labels) == 0 {
		return []map[string]interface{}{}
	}

	result := make([]map[string]interface{}, 0, len(labels))
	for _, label := range labels {
		item := map[string]interface{}{
			"src": label.Src,
			"uri": label.URI,
			"cid": nil,
			"val": label.Val,
			"neg": label.Neg,
			"cts": label.Cts,
			"exp": nil,
			"ver": nil,
		}
		if label.CID != nil {
			item["cid"] = *label.CID
		}
		if label.Exp != nil {
			item["exp"] = *label.Exp
		}
		if label.Ver != nil {
			item["ver"] = int(*label.Ver)
		}
		result = append(result, item)
	}
	return result
}

func stringSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func stringListArg(value interface{}) []string {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case []interface{}:
		strings := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				strings = append(strings, s)
			}
		}
		return strings
	case []string:
		return v
	default:
		return nil
	}
}
