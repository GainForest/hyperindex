// Package recordhistory exposes the append-only record version history kept
// for collections opted in with RECORD_HISTORY_COLLECTIONS.
package recordhistory

import (
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"github.com/graphql-go/graphql"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/graphql/types"
)

// DefaultPageSize is the number of versions returned when `first` is omitted.
const DefaultPageSize = 100

// Type is one observed version of a record.
var Type = graphql.NewObject(graphql.ObjectConfig{
	Name:        "RecordVersion",
	Description: "One observed version of a record (oldest first). Only kept for collections the indexer is configured to track; deleting the record or its account removes its history.",
	Fields: graphql.Fields{
		"id": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "Monotonic version id; later versions have larger ids.",
		},
		"uri":        &graphql.Field{Type: graphql.NewNonNull(graphql.String), Description: "Record AT-URI."},
		"cid":        &graphql.Field{Type: graphql.NewNonNull(graphql.String), Description: "CID of this version."},
		"did":        &graphql.Field{Type: graphql.NewNonNull(graphql.String), Description: "Repository DID."},
		"collection": &graphql.Field{Type: graphql.NewNonNull(graphql.String), Description: "Collection NSID."},
		"action": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "baseline (current when history was switched on), create, or update.",
		},
		"value": &graphql.Field{
			Type:        types.JSONScalar,
			Description: "The record body at this version.",
		},
		"live": &graphql.Field{
			Type:        graphql.Boolean,
			Description: "True when seen on the live stream, false for a resync delivery, null for baseline rows.",
		},
		"observedAt": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "When the indexer observed this version (RFC 3339). For baseline rows, when the version was indexed.",
		},
	},
})

// ToGraphQL converts versions to GraphQL result maps.
func ToGraphQL(versions []repositories.RecordVersion) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(versions))
	for _, v := range versions {
		var value interface{}
		if v.JSON != nil {
			if err := json.Unmarshal([]byte(*v.JSON), &value); err != nil {
				slog.Warn("Record version has invalid JSON", "uri", v.URI, "id", v.ID, "error", err)
				value = nil
			}
		}
		var live interface{}
		if v.Live != nil {
			live = *v.Live
		}
		out = append(out, map[string]interface{}{
			"id":         strconv.FormatInt(v.ID, 10),
			"uri":        v.URI,
			"cid":        v.CID,
			"did":        v.DID,
			"collection": v.Collection,
			"action":     v.Action,
			"value":      value,
			"live":       live,
			"observedAt": v.ObservedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return out
}
