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
	OutputFormat string `json:"output_format,omitempty" mcp:"optional format to store result sets on disk: csv or json; requires output_path"`
	OutputPath   string `json:"output_path,omitempty" mcp:"optional file path to store the result sets in the given output_format; multiple result sets are written to numbered files"`
}

// ExecuteResult is the result of the execute tool.
type ExecuteResult struct {
	Results    []QueryResult `json:"results"`
	SavedFiles []string      `json:"saved_files,omitempty" mcp:"paths of the files the result sets were written to"`
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

	if err := validateExecuteArgs(&args); err != nil {
		return nil, ExecuteResult{}, err
	}

	if args.OutputFormat != "" {
		if err := outputGuard().checkWritable(args.OutputPath); err != nil {
			return nil, ExecuteResult{}, err
		}
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

	text := resultsText(results)
	var saved []string
	if args.OutputFormat != "" {
		saved, err = writeOutput(args.OutputPath, args.OutputFormat, results)
		if err != nil {
			return nil, ExecuteResult{}, err
		}
		text += "\n\nsaved output to: " + strings.Join(saved, ", ")
	}

	res.Content = []mcp.Content{&mcp.TextContent{Text: text}}
	return res, ExecuteResult{Results: results, SavedFiles: saved}, nil
}

// resultsText renders the result sets as readable text so agents see the
// actual query output, not just a success message.
func resultsText(results []QueryResult) string {
	if len(results) == 0 {
		return "query executed successfully"
	}

	var b strings.Builder
	for i, r := range results {
		if i > 0 {
			b.WriteString("\n")
		}
		if len(results) > 1 {
			fmt.Fprintf(&b, "result %d:\n", i+1)
		}
		if len(r.Columns) == 0 {
			fmt.Fprintf(&b, "rows affected: %d", r.RowsAffected)
			continue
		}
		b.WriteString(strings.Join(r.Columns, "\t"))
		b.WriteString("\n")
		for _, row := range r.Rows {
			for j, v := range row {
				if j > 0 {
					b.WriteString("\t")
				}
				b.WriteString(fmt.Sprintf("%v", v))
			}
			b.WriteString("\n")
		}
		if len(r.Rows) == 0 {
			b.WriteString("(no rows)")
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func validateExecuteArgs(args *ExecuteArgs) error {
	if args.ConnectionID == "" {
		return errors.New("connection_id is required: call the connect method first")
	}
	if args.Query == "" {
		return errors.New("query is required")
	}

	args.OutputFormat = strings.ToLower(strings.TrimSpace(args.OutputFormat))
	switch args.OutputFormat {
	case "", "csv", "json":
	default:
		return fmt.Errorf("output_format must be csv or json, got %q", args.OutputFormat)
	}
	if args.OutputFormat == "" && args.OutputPath != "" {
		return errors.New("output_format is required when output_path is set")
	}
	if args.OutputFormat != "" && args.OutputPath == "" {
		return errors.New("output_path is required when output_format is set")
	}
	return nil
}

// runQuery executes a SQL batch. Statements are split on top-level
// semicolons and run one at a time. Queries that return rows (SELECT,
// SHOW, EXPLAIN, DESCRIBE, PRAGMA, WITH ...) produce a result set with
// columns and rows; other statements produce a result with a row count.
// Params are bound to each statement by placeholder count, and any
// unused params are forwarded as-is.
func runQuery(ctx context.Context, db *sql.DB, query string, params []any) ([]QueryResult, error) {
	stmts := splitStatements(query)
	if len(stmts) == 0 {
		return nil, errors.New("query contains no statements")
	}

	results := make([]QueryResult, 0, len(stmts))
	for _, stmt := range stmts {
		r, err := execOne(ctx, db, stmt, params)
		if err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

func execOne(ctx context.Context, db *sql.DB, stmt string, params []any) (QueryResult, error) {
	if likelyReturnsRows(stmt) {
		return queryResult(ctx, db, stmt, params)
	}

	n := placeholderCount(stmt)
	if n < len(params) {
		params = params[:n]
	}
	r, err := db.ExecContext(ctx, stmt, params...)
	if err != nil {
		return QueryResult{}, fmt.Errorf("executing query: %w", err)
	}
	affected, _ := r.RowsAffected()
	return QueryResult{RowsAffected: affected}, nil
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
