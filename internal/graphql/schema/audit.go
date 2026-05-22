package schema

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/graphql-go/graphql"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/graphql/query"
	"github.com/GainForest/hyperindex/internal/graphql/resolver"
	"github.com/GainForest/hyperindex/internal/graphql/types"
)

var auditRecordActionEnum = graphql.NewEnum(graphql.EnumConfig{
	Name:        "AuditRecordAction",
	Description: "The append-only audit action recorded for an AT Protocol record.",
	Values: graphql.EnumValueConfigMap{
		"CREATE": &graphql.EnumValueConfig{Value: "create", Description: "The record was created."},
		"UPDATE": &graphql.EnumValueConfig{Value: "update", Description: "The record was updated."},
		"DELETE": &graphql.EnumValueConfig{Value: "delete", Description: "The record was deleted from the current projection."},
	},
})

var auditStringFilterInput = graphql.NewInputObject(graphql.InputObjectConfig{
	Name:        "AuditStringFilterInput",
	Description: "Filter an audit field by exact string equality.",
	Fields: graphql.InputObjectConfigFieldMap{
		"eq": &graphql.InputObjectFieldConfig{Type: graphql.String, Description: "Exact string to match."},
	},
})

var auditIntFilterInput = graphql.NewInputObject(graphql.InputObjectConfig{
	Name:        "AuditIntFilterInput",
	Description: "Filter an audit field by exact integer equality.",
	Fields: graphql.InputObjectConfigFieldMap{
		"eq": &graphql.InputObjectFieldConfig{Type: graphql.Int, Description: "Exact integer to match."},
	},
})

var auditBooleanFilterInput = graphql.NewInputObject(graphql.InputObjectConfig{
	Name:        "AuditBooleanFilterInput",
	Description: "Filter an audit field by exact boolean equality.",
	Fields: graphql.InputObjectConfigFieldMap{
		"eq": &graphql.InputObjectFieldConfig{Type: graphql.Boolean, Description: "Exact boolean to match."},
	},
})

var auditDateTimeFilterInput = graphql.NewInputObject(graphql.InputObjectConfig{
	Name:        "AuditDateTimeFilterInput",
	Description: "Filter an audit timestamp by equality or open-ended range.",
	Fields: graphql.InputObjectConfigFieldMap{
		"eq": &graphql.InputObjectFieldConfig{Type: types.DateTimeScalar, Description: "Exact timestamp to match."},
		"gt": &graphql.InputObjectFieldConfig{Type: types.DateTimeScalar, Description: "Only events after this timestamp."},
		"lt": &graphql.InputObjectFieldConfig{Type: types.DateTimeScalar, Description: "Only events before this timestamp."},
	},
})

var auditRecordActionFilterInput = graphql.NewInputObject(graphql.InputObjectConfig{
	Name:        "AuditRecordActionFilterInput",
	Description: "Filter audit record events by action.",
	Fields: graphql.InputObjectConfigFieldMap{
		"eq": &graphql.InputObjectFieldConfig{Type: auditRecordActionEnum, Description: "Exact audit action to match."},
	},
})

var auditRecordEventWhereInput = graphql.NewInputObject(graphql.InputObjectConfig{
	Name:        "AuditRecordEventWhere",
	Description: "Filter conditions for append-only audit record event queries.",
	Fields: graphql.InputObjectConfigFieldMap{
		"id":         &graphql.InputObjectFieldConfig{Type: auditIntFilterInput, Description: "Filter by record_events.id."},
		"uri":        &graphql.InputObjectFieldConfig{Type: auditStringFilterInput, Description: "Filter by AT-URI."},
		"did":        &graphql.InputObjectFieldConfig{Type: auditStringFilterInput, Description: "Filter by repository DID."},
		"collection": &graphql.InputObjectFieldConfig{Type: auditStringFilterInput, Description: "Filter by collection NSID."},
		"rkey":       &graphql.InputObjectFieldConfig{Type: auditStringFilterInput, Description: "Filter by record key."},
		"action":     &graphql.InputObjectFieldConfig{Type: auditRecordActionFilterInput, Description: "Filter by create, update, or delete action."},
		"live":       &graphql.InputObjectFieldConfig{Type: auditBooleanFilterInput, Description: "Filter by Tap live/backfill marker."},
		"rev":        &graphql.InputObjectFieldConfig{Type: auditStringFilterInput, Description: "Filter by repository revision."},
		"cid":        &graphql.InputObjectFieldConfig{Type: auditStringFilterInput, Description: "Filter by record CID; empty string matches missing CIDs."},
		"receivedAt": &graphql.InputObjectFieldConfig{Type: auditDateTimeFilterInput, Description: "Filter by receive timestamp."},
	},
})

var auditRecordEventOrderFieldEnum = graphql.NewEnum(graphql.EnumConfig{
	Name:        "AuditRecordEventOrderField",
	Description: "Fields available for stable audit record event ordering.",
	Values: graphql.EnumValueConfigMap{
		"ID": &graphql.EnumValueConfig{Value: "id", Description: "Order by record_events.id."},
	},
})

var auditRecordEventOrderInput = graphql.NewInputObject(graphql.InputObjectConfig{
	Name:        "AuditRecordEventOrder",
	Description: "Ordering for audit record event queries. Cursors are stable record_events.id cursors.",
	Fields: graphql.InputObjectConfigFieldMap{
		"field":     &graphql.InputObjectFieldConfig{Type: auditRecordEventOrderFieldEnum, Description: "Field to order by. Only ID is supported."},
		"direction": &graphql.InputObjectFieldConfig{Type: query.SortDirectionEnum, Description: "Sort direction, defaulting to DESC."},
	},
})

var auditRecordEventType = graphql.NewObject(graphql.ObjectConfig{
	Name:        "AuditRecordEvent",
	Description: "An immutable append-only audit event for an AT Protocol record observed through Tap.",
	Fields: graphql.Fields{
		"id": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.ID),
			Description: "Stable record_events row id used for audit cursors.",
		},
		"receivedAt": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "Database timestamp when the Tap delivery was stored.",
		},
		"live": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.Boolean),
			Description: "Tap live/backfill marker; false means Tap emitted the event from backfill or resync.",
		},
		"rev": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "Repository revision associated with this record change, empty when Tap omitted it.",
		},
		"did": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "DID of the repository that owns the record.",
		},
		"collection": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "AT Protocol collection NSID.",
		},
		"rkey": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "Record key within the collection.",
		},
		"uri": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "AT-URI of the audited record.",
		},
		"action": &graphql.Field{
			Type:        graphql.NewNonNull(auditRecordActionEnum),
			Description: "Create, update, or delete action captured in the audit ledger.",
		},
		"cid": &graphql.Field{
			Type:        graphql.String,
			Description: "Content identifier for create/update events when Tap provided one.",
		},
		"record": &graphql.Field{
			Type:        types.JSONScalar,
			Description: "Decoded JSON record body for create/update events, or null for deletes and missing bodies.",
		},
	},
})

var auditRecordEventConnectionType = query.BuildConnectionType(auditRecordEventType)

const maxGraphQLInt = int64(1<<31 - 1)

func auditRecordEventArgs() graphql.FieldConfigArgument {
	return graphql.FieldConfigArgument{
		"first": &graphql.ArgumentConfig{
			Type:         graphql.Int,
			DefaultValue: repositories.DefaultAuditRecordEventPageSize,
			Description:  fmt.Sprintf("Number of audit events to return (default %d, max %d).", repositories.DefaultAuditRecordEventPageSize, repositories.MaxAuditRecordEventPageSize),
		},
		"after": &graphql.ArgumentConfig{
			Type:        graphql.String,
			Description: "Opaque audit cursor returned from a previous page.",
		},
		"where": &graphql.ArgumentConfig{
			Type:        auditRecordEventWhereInput,
			Description: "Filter conditions for audit record events.",
		},
		"orderBy": &graphql.ArgumentConfig{
			Type:        auditRecordEventOrderInput,
			Description: "Stable ID ordering for audit record events.",
		},
	}
}

func (b *Builder) createAuditRecordEventsResolver() graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		repos := resolver.GetRepositories(p.Context)
		if repos == nil || repos.Audit == nil {
			return emptyConnection(), nil
		}

		opts, err := auditRecordEventFindOptionsFromArgs(p.Args)
		if err != nil {
			return nil, err
		}

		page, err := repos.Audit.FindRecordEvents(p.Context, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to query audit record events: %w", err)
		}

		connection, err := auditRecordEventConnection(page)
		if err != nil {
			return nil, err
		}

		if isTotalCountRequested(p) {
			count, err := repos.Audit.CountRecordEvents(p.Context, opts.Where)
			if err != nil {
				return nil, fmt.Errorf("failed to count audit record events: %w", err)
			}
			if count > maxGraphQLInt {
				return nil, fmt.Errorf("auditRecordEvents totalCount %d exceeds GraphQL Int maximum %d; add filters or omit totalCount", count, maxGraphQLInt)
			}
			connection["totalCount"] = int(count)
		}

		return connection, nil
	}
}

func auditRecordEventFindOptionsFromArgs(args map[string]interface{}) (repositories.RecordEventFindOptions, error) {
	var opts repositories.RecordEventFindOptions
	if first, ok := args["first"].(int); ok {
		opts.First = first
	}
	if after, ok := args["after"].(string); ok {
		opts.After = after
	}
	if whereArg, ok := args["where"].(map[string]interface{}); ok {
		filters, err := auditRecordEventFiltersFromArg(whereArg)
		if err != nil {
			return opts, err
		}
		opts.Where = filters
	}
	if orderArg, ok := args["orderBy"].(map[string]interface{}); ok {
		order, err := auditRecordEventOrderFromArg(orderArg)
		if err != nil {
			return opts, err
		}
		opts.OrderBy = order
	}
	return opts, nil
}

func auditRecordEventFiltersFromArg(where map[string]interface{}) (repositories.AuditRecordEventFilters, error) {
	var filters repositories.AuditRecordEventFilters

	if value, ok, err := auditIntEq(where, "id"); err != nil {
		return filters, err
	} else if ok {
		filters.ID = &value
	}
	if value, ok, err := auditStringEq(where, "uri"); err != nil {
		return filters, err
	} else if ok {
		filters.URI = &value
	}
	if value, ok, err := auditStringEq(where, "did"); err != nil {
		return filters, err
	} else if ok {
		filters.DID = &value
	}
	if value, ok, err := auditStringEq(where, "collection"); err != nil {
		return filters, err
	} else if ok {
		filters.Collection = &value
	}
	if value, ok, err := auditStringEq(where, "rkey"); err != nil {
		return filters, err
	} else if ok {
		filters.RKey = &value
	}
	if value, ok, err := auditStringEq(where, "action"); err != nil {
		return filters, err
	} else if ok {
		value = strings.ToLower(strings.TrimSpace(value))
		filters.Action = &value
	}
	if value, ok, err := auditBoolEq(where, "live"); err != nil {
		return filters, err
	} else if ok {
		filters.Live = &value
	}
	if value, ok, err := auditStringEq(where, "rev"); err != nil {
		return filters, err
	} else if ok {
		filters.Rev = &value
	}
	if value, ok, err := auditStringEq(where, "cid"); err != nil {
		return filters, err
	} else if ok {
		filters.CID = &value
	}
	if receivedAt, ok := where["receivedAt"].(map[string]interface{}); ok {
		if value, ok, err := auditFilterStringValue(receivedAt, "eq", "receivedAt"); err != nil {
			return filters, err
		} else if ok {
			filters.ReceivedAt = &value
		}
		if value, ok, err := auditFilterStringValue(receivedAt, "gt", "receivedAt"); err != nil {
			return filters, err
		} else if ok {
			filters.ReceivedAtAfter = &value
		}
		if value, ok, err := auditFilterStringValue(receivedAt, "lt", "receivedAt"); err != nil {
			return filters, err
		} else if ok {
			filters.ReceivedAtBefore = &value
		}
	}

	return filters, nil
}

func auditRecordEventOrderFromArg(order map[string]interface{}) (repositories.AuditRecordEventOrder, error) {
	if field, ok := order["field"].(string); ok && field != "" && field != "id" {
		return repositories.AuditRecordEventOrder{}, fmt.Errorf("unsupported audit record event order field %q; expected ID", field)
	}
	direction, _ := order["direction"].(string)
	return repositories.AuditRecordEventOrder{Direction: direction}, nil
}

func auditRecordEventConnection(page *repositories.AuditRecordEventPage) (map[string]interface{}, error) {
	if page == nil {
		return emptyConnection(), nil
	}

	edges := make([]interface{}, 0, len(page.Edges))
	for _, edge := range page.Edges {
		if edge == nil || edge.Node == nil {
			continue
		}
		node, err := auditRecordEventNode(edge.Node)
		if err != nil {
			return nil, err
		}
		edges = append(edges, map[string]interface{}{
			"cursor": edge.Cursor,
			"node":   node,
		})
	}

	return map[string]interface{}{
		"edges": edges,
		"pageInfo": map[string]interface{}{
			"hasNextPage":     page.HasNextPage,
			"hasPreviousPage": page.HasPreviousPage,
			"startCursor":     optionalString(page.StartCursor),
			"endCursor":       optionalString(page.EndCursor),
		},
	}, nil
}

func auditRecordEventNode(event *repositories.AuditRecordEvent) (map[string]interface{}, error) {
	record, err := auditRecordJSON(event)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"id":         strconv.FormatInt(event.ID, 10),
		"receivedAt": event.ReceivedAt,
		"live":       event.Live,
		"rev":        event.Rev,
		"did":        event.DID,
		"collection": event.Collection,
		"rkey":       event.RKey,
		"uri":        event.URI,
		"action":     event.Action,
		"cid":        optionalString(event.CID),
		"record":     record,
	}, nil
}

func auditRecordJSON(event *repositories.AuditRecordEvent) (interface{}, error) {
	if event.Record == nil {
		return nil, nil
	}

	var record interface{}
	decoder := json.NewDecoder(strings.NewReader(*event.Record))
	decoder.UseNumber()
	if err := decoder.Decode(&record); err != nil {
		return nil, fmt.Errorf("failed to decode audit record JSON for event %d: %w", event.ID, err)
	}
	return record, nil
}

func optionalString(value *string) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func auditStringEq(where map[string]interface{}, field string) (string, bool, error) {
	filter, ok := where[field].(map[string]interface{})
	if !ok {
		return "", false, nil
	}
	return auditFilterStringValue(filter, "eq", field)
}

func auditFilterStringValue(filter map[string]interface{}, operator, field string) (string, bool, error) {
	value, ok := filter[operator]
	if !ok || value == nil {
		return "", false, nil
	}
	text, ok := value.(string)
	if !ok {
		return "", false, fmt.Errorf("audit filter %s.%s must be a string", field, operator)
	}
	return text, true, nil
}

func auditIntEq(where map[string]interface{}, field string) (int64, bool, error) {
	filter, ok := where[field].(map[string]interface{})
	if !ok {
		return 0, false, nil
	}
	value, ok := filter["eq"]
	if !ok || value == nil {
		return 0, false, nil
	}
	intValue, ok := value.(int)
	if !ok {
		return 0, false, fmt.Errorf("audit filter %s.eq must be an integer", field)
	}
	return int64(intValue), true, nil
}

func auditBoolEq(where map[string]interface{}, field string) (bool, bool, error) {
	filter, ok := where[field].(map[string]interface{})
	if !ok {
		return false, false, nil
	}
	value, ok := filter["eq"]
	if !ok || value == nil {
		return false, false, nil
	}
	boolValue, ok := value.(bool)
	if !ok {
		return false, false, fmt.Errorf("audit filter %s.eq must be a boolean", field)
	}
	return boolValue, true, nil
}
