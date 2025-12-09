package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	mcpgrafana "github.com/grafana/mcp-grafana"

	"github.com/grafana/grafana-openapi-client-go/client/annotations"
)

// GetAnnotationsInput filters annotation search.
type GetAnnotationsInput struct {
	From         *int64   `jsonschema:"description=Epoch ms start time"`
	To           *int64   `jsonschema:"description=Epoch ms end time"`
	Limit        *int64   `jsonschema:"description=Max results default 100"`
	AlertID      *int64   `jsonschema:"description=Deprecated. Use AlertUID"`
	AlertUID     *string  `jsonschema:"description=Filter by alert UID"`
	DashboardID  *int64   `jsonschema:"description=Deprecated. Use DashboardUID"`
	DashboardUID *string  `jsonschema:"description=Filter by dashboard UID"`
	PanelID      *int64   `jsonschema:"description=Filter by panel ID"`
	UserID       *int64   `jsonschema:"description=Filter by creator user ID"`
	Type         *string  `jsonschema:"description=annotation or alert"`
	Tags         []string `jsonschema:"description=Multiple tags allowed tags=tag1&tags=tag2"`
	MatchAny     *bool    `jsonschema:"description=true OR tag match false AND"`
}

// getAnnotations retrieves Grafana annotations using filters.
func getAnnotations(ctx context.Context, args GetAnnotationsInput) (*annotations.GetAnnotationsOK, error) {
	c := mcpgrafana.GrafanaClientFromContext(ctx)

	req := annotations.GetAnnotationsParams{
		From:         args.From,
		To:           args.To,
		Limit:        args.Limit,
		AlertID:      args.AlertID,
		AlertUID:     args.AlertUID,
		DashboardID:  args.DashboardID,
		DashboardUID: args.DashboardUID,
		PanelID:      args.PanelID,
		UserID:       args.UserID,
		Type:         args.Type,
		Tags:         args.Tags,
		MatchAny:     args.MatchAny,
		Context:      ctx,
	}

	resp, err := c.Annotations.GetAnnotations(&req)
	if err != nil {
		return nil, fmt.Errorf("get annotations: %w", err)
	}

	return resp, nil
}

var GetAnnotationsTool = mcpgrafana.MustTool(
	"get_annotations",
	"Fetch Grafana annotations using filters such as dashboard UID, time range and tags.",
	getAnnotations,
	mcp.WithTitleAnnotation("Get Annotations"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// GetAnnotationTagsInput defines filters for retrieving annotation tags.
type GetAnnotationTagsInput struct {
	Tag   *string `json:"tag,omitempty"   jsonschema:"description=Optional filter by tag name"`
	Limit *string `json:"limit,omitempty" jsonschema:"description=Max results\\, default 100"`
}

func getAnnotationTags(ctx context.Context, args GetAnnotationTagsInput) (*annotations.GetAnnotationTagsOK, error) {
	c := mcpgrafana.GrafanaClientFromContext(ctx)

	req := annotations.GetAnnotationTagsParams{
		Tag:     args.Tag,
		Limit:   args.Limit,
		Context: ctx,
	}

	resp, err := c.Annotations.GetAnnotationTags(&req)
	if err != nil {
		return nil, fmt.Errorf("get annotation tags: %w", err)
	}

	return resp, nil
}

var GetAnnotationTagsTool = mcpgrafana.MustTool(
	"get_annotation_tags",
	"Returns annotation tags with optional filtering by tag name. Only the provided filters are applied.",
	getAnnotationTags,
	mcp.WithTitleAnnotation("Get Annotation Tags"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

func AddAnnotationTools(mcp *server.MCPServer) {
	GetAnnotationsTool.Register(mcp)
	GetAnnotationTagsTool.Register(mcp)
}
