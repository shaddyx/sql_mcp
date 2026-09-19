package tools

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// writeOutput exports every result set that carries rows to files in the
// requested format ("csv" or "json") and returns the written paths. When
// several result sets exist, each one is written to a numbered file derived
// from path (out.csv becomes out_1.csv, out_2.csv, ...).
func writeOutput(path, format string, results []QueryResult) ([]string, error) {
	sets := make([]QueryResult, 0, len(results))
	for _, r := range results {
		if len(r.Columns) > 0 {
			sets = append(sets, r)
		}
	}
	if len(sets) == 0 {
		return nil, errors.New("output requested but the query produced no result sets")
	}

	paths := outputPaths(path, len(sets))
	written := make([]string, 0, len(paths))
	for i, p := range paths {
		var err error
		switch format {
		case "csv":
			err = writeCSV(p, sets[i])
		case "json":
			err = writeJSON(p, sets[i])
		default:
			return nil, fmt.Errorf("unsupported output_format %q", format)
		}
		if err != nil {
			return written, err
		}
		written = append(written, p)
	}
	return written, nil
}

// outputPaths returns the destination files for n result sets. A single set
// uses path as-is; multiple sets get a numeric suffix before the extension.
func outputPaths(path string, n int) []string {
	if n == 1 {
		return []string{path}
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	paths := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		paths = append(paths, fmt.Sprintf("%s_%d%s", base, i, ext))
	}
	return paths
}

func writeCSV(path string, r QueryResult) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write(r.Columns); err != nil {
		return fmt.Errorf("writing csv header: %w", err)
	}
	for _, row := range r.Rows {
		rec := make([]string, len(row))
		for i, v := range row {
			rec[i] = csvValue(v)
		}
		if err := w.Write(rec); err != nil {
			return fmt.Errorf("writing csv row: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("writing csv output: %w", err)
	}
	return f.Close()
}

// writeJSON writes the result set as a JSON array of objects keyed by
// column name. Object keys keep the column order of the query.
func writeJSON(path string, r QueryResult) error {
	var b bytes.Buffer
	b.WriteByte('[')
	for i, row := range r.Rows {
		if i > 0 {
			b.WriteByte(',')
		}
		if err := writeJSONObject(&b, r.Columns, row); err != nil {
			return err
		}
	}
	b.WriteByte(']')

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		return fmt.Errorf("writing json output: %w", err)
	}
	return nil
}

func writeJSONObject(b *bytes.Buffer, cols []string, row []any) error {
	b.WriteByte('{')
	for i, col := range cols {
		if i > 0 {
			b.WriteByte(',')
		}
		key, err := json.Marshal(col)
		if err != nil {
			return fmt.Errorf("encoding column name %q: %w", col, err)
		}
		b.Write(key)
		b.WriteByte(':')
		var val any
		if i < len(row) {
			val = row[i]
		}
		v, err := json.Marshal(val)
		if err != nil {
			return fmt.Errorf("encoding value for column %q: %w", col, err)
		}
		b.Write(v)
	}
	b.WriteByte('}')
	return nil
}

// csvValue renders a database value as a CSV-safe string.
func csvValue(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case time.Time:
		return t.Format(time.RFC3339Nano)
	case []byte:
		return string(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}
