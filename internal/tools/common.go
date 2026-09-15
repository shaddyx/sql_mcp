package tools

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"sql-tool/internal/manager"
)

var errManagerMissing = errors.New("handler is not bound to a connection manager")

// Handler holds the connection manager and provides the MCP tool handlers.
type Handler struct {
	m *manager.Manager
}

// NewHandler returns a Handler bound to the given manager.
func NewHandler(m *manager.Manager) *Handler {
	return &Handler{m: m}
}

// ConnectArgs are the parameters for the connect tool.
type ConnectArgs struct {
	ConnectionURL string `json:"connection_url" mcp:"the connection url specifying the database location and authentication details, e.g. postgresql://user:password@localhost:5432/mydatabase"`
}

// ConnectResult is the result of the connect tool.
type ConnectResult struct {
	ConnectionID string `json:"connection_id" mcp:"the id of the established database connection"`
}

// Connect implements the connect tool.
func (h *Handler) Connect(ctx context.Context, _ *mcp.CallToolRequest, args ConnectArgs) (*mcp.CallToolResult, ConnectResult, error) {
	res := &mcp.CallToolResult{}
	if args.ConnectionURL == "" {
		return nil, ConnectResult{}, errors.New("connection_url is required")
	}

	cid, err := h.m.Connect(ctx, args.ConnectionURL)
	if err != nil {
		return nil, ConnectResult{}, err
	}

	res.Content = []mcp.Content{&mcp.TextContent{Text: "connected to database: " + cid}}
	return res, ConnectResult{ConnectionID: cid}, nil
}
