package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ExecuteArgs are the parameters for the execute tool.
type ExecuteArgs struct {
	ConnectionID string `json:"connection_id" mcp:"the id of the established database connection"`
	Query        string `json:"query" mcp:"the sql query to execute"`
	Params       []any  `json:"params,omitempty" mcp:"optional parameters to bind to the sql query"`
	Timeout      *int   `json:"timeout,omitempty" mcp:"maximum time in seconds to wait for the query, default 30"`
}

// ExecuteResult is the result of the execute tool.
type ExecuteResult struct {
	Results []QueryResult `json:"results"`
}

// QueryResult is a single result set.
type QueryResult struct {
	Columns      []string `json:"columns,omitempty"`
	Rows         [][]any  `json:"rows,omitempty"`
	RowsAffected int64    `json:"rows_affected,omitempty"`
}

// DefaultTimeoutSeconds is used when the caller does not provide a timeout.
const DefaultTimeoutSeconds = 30

// Execute implements the execute tool.
func (h *Handler) Execute(ctx context.Context, _ *mcp.CallToolRequest, args ExecuteArgs) (*mcp.CallToolResult, ExecuteResult, error) {
	res := &mcp.CallToolResult{}

	if err := validateExecuteArgs(args); err != nil {
		return nil, ExecuteResult{}, err
	}

	timeout := DefaultTimeoutSeconds
	if args.Timeout != nil && *args.Timeout > 0 {
		timeout = *args.Timeout
	}

	ac, err := h.m.ActiveConn(args.ConnectionID)
	if err != nil {
		return nil, ExecuteResult{}, err
	}
	defer ac.Done()

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	results, err := runQuery(ctx, ac.DB(), args.Query, args.Params)
	if err != nil {
		return nil, ExecuteResult{}, err
	}

	res.Content = []mcp.Content{&mcp.TextContent{Text: "query executed successfully"}}
	return res, ExecuteResult{Results: results}, nil
}

func validateExecuteArgs(args ExecuteArgs) error {
	if args.ConnectionID == "" {
		return errors.New("connection_id is required: call the connect method first")
	}
	if args.Query == "" {
		return errors.New("query is required")
	}
	return nil
}

// runQuery executes one SQL statement. Queries that return rows (SELECT,
// SHOW, EXPLAIN, DESCRIBE, PRAGMA, WITH ...) produce a result set with
// columns and rows; other statements produce a result with a row count.
func runQuery(ctx context.Context, db *sql.DB, query string, params []any) ([]QueryResult, error) {
	if likelyReturnsRows(query) {
		r, err := queryResult(ctx, db, query, params)
		if err != nil {
			return nil, err
		}
		return []QueryResult{r}, nil
	}

	r, err := db.ExecContext(ctx, query, params...)
	if err != nil {
		return nil, fmt.Errorf("executing query: %w", err)
	}
	affected, _ := r.RowsAffected()
	return []QueryResult{{RowsAffected: affected}}, nil
}

func queryResult(ctx context.Context, db *sql.DB, query string, params []any) (QueryResult, error) {
	rows, err := db.QueryContext(ctx, query, params...)
	if err != nil {
		return QueryResult{}, fmt.Errorf("executing query: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return QueryResult{}, fmt.Errorf("reading result columns: %w", err)
	}

	result := QueryResult{Columns: cols}
	for rows.Next() {
		values := make([]any, len(cols))
		scanArgs := make([]any, len(cols))
		for i := range values {
			scanArgs[i] = &values[i]
		}
		if err := rows.Scan(scanArgs...); err != nil {
			return QueryResult{}, fmt.Errorf("scanning row: %w", err)
		}

		result.Rows = append(result.Rows, convertRow(values))
	}
	if err := rows.Err(); err != nil {
		return QueryResult{}, fmt.Errorf("iterating rows: %w", err)
	}

	return result, nil
}

// convertRow converts raw sql.RawBytes into JSON-friendly values.
func convertRow(raw []any) []any {
	out := make([]any, len(raw))
	for i, v := range raw {
		switch t := v.(type) {
		case nil:
			out[i] = nil
		case []byte:
			var f float64
			if json.Unmarshal(t, &f) == nil {
				out[i] = f
			} else {
				out[i] = string(t)
			}
		case time.Time:
			out[i] = t
		default:
			out[i] = v
		}
	}
	return out
}

// likelyReturnsRows reports whether the statement is expected to produce a
// result set based on its leading keyword.
func likelyReturnsRows(query string) bool {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return false
	}

	upper := strings.ToUpper(trimmed)
	for _, prefix := range []string{"SELECT", "SHOW", "EXPLAIN", "DESCRIBE", "PRAGMA", "WITH"} {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	return false
}
