package tools

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DisconnectArgs are the parameters for the disconnect tool.
type DisconnectArgs struct {
	ConnectionID string `json:"connection_id" mcp:"the id of the established database connection to be closed"`
}

// Disconnect implements the disconnect tool.
func (h *Handler) Disconnect(ctx context.Context, _ *mcp.CallToolRequest, args DisconnectArgs) (*mcp.CallToolResult, any, error) {
	res := &mcp.CallToolResult{}

	if args.ConnectionID == "" {
		return nil, nil, errors.New("connection_id is required")
	}

	if err := h.m.Disconnect(args.ConnectionID); err != nil {
		return nil, nil, err
	}

	res.Content = []mcp.Content{&mcp.TextContent{Text: "connection closed"}}
	return res, nil, nil
}
