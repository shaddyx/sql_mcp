package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOutputPaths(t *testing.T) {
	tests := []struct {
		name string
		path string
		n    int
		want []string
	}{
		{
			name: "single set keeps path as-is",
			path: "/tmp/out.csv",
			n:    1,
			want: []string{"/tmp/out.csv"},
		},
		{
			name: "multiple sets are numbered before the extension",
			path: "/tmp/out.json",
			n:    3,
			want: []string{"/tmp/out_1.json", "/tmp/out_2.json", "/tmp/out_3.json"},
		},
		{
			name: "path without extension",
			path: "/tmp/out",
			n:    2,
			want: []string{"/tmp/out_1", "/tmp/out_2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := outputPaths(tt.path, tt.n)
			if len(got) != len(tt.want) {
				t.Fatalf("outputPaths(%q, %d) = %#v, want %#v", tt.path, tt.n, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("outputPaths(%q, %d)[%d] = %q, want %q", tt.path, tt.n, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestWriteOutput(t *testing.T) {
	setA := QueryResult{
		Columns: []string{"id", "name"},
		Rows:    [][]any{{int64(1), "alice"}, {int64(2), "bob"}},
	}
	setB := QueryResult{
		Columns: []string{"total"},
		Rows:    [][]any{{int64(2)}},
	}

	t.Run("csv single set", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "out.csv")
		saved, err := writeOutput(path, "csv", []QueryResult{setA})
		if err != nil {
			t.Fatalf("writeOutput() error = %v", err)
		}
		if len(saved) != 1 || saved[0] != path {
			t.Fatalf("writeOutput() saved = %#v, want [%s]", saved, path)
		}
		got := readFile(t, path)
		want := "id,name\n1,alice\n2,bob\n"
		if got != want {
			t.Fatalf("csv content = %q, want %q", got, want)
		}
	})

	t.Run("csv with nil and time values", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "out.csv")
		ts := time.Date(2026, 9, 20, 10, 30, 0, 0, time.UTC)
		set := QueryResult{
			Columns: []string{"v", "t"},
			Rows:    [][]any{{nil, ts}},
		}
		if _, err := writeOutput(path, "csv", []QueryResult{set}); err != nil {
			t.Fatalf("writeOutput() error = %v", err)
		}
		got := readFile(t, path)
		want := "v,t\n,2026-09-20T10:30:00Z\n"
		if got != want {
			t.Fatalf("csv content = %q, want %q", got, want)
		}
	})

	t.Run("json single set keeps column order", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "out.json")
		if _, err := writeOutput(path, "json", []QueryResult{setA}); err != nil {
			t.Fatalf("writeOutput() error = %v", err)
		}
		got := readFile(t, path)
		want := `[{"id":1,"name":"alice"},{"id":2,"name":"bob"}]`
		if got != want {
			t.Fatalf("json content = %s, want %s", got, want)
		}
	})

	t.Run("json decodes back to objects", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "out.json")
		if _, err := writeOutput(path, "json", []QueryResult{setA}); err != nil {
			t.Fatalf("writeOutput() error = %v", err)
		}
		var rows []map[string]any
		if err := json.Unmarshal(readFileBytes(t, path), &rows); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if len(rows) != 2 || rows[0]["name"] != "alice" || rows[1]["id"] != float64(2) {
			t.Fatalf("decoded rows = %#v", rows)
		}
	})

	t.Run("multiple sets are written to numbered files", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "out.csv")
		saved, err := writeOutput(path, "csv", []QueryResult{setA, setB})
		if err != nil {
			t.Fatalf("writeOutput() error = %v", err)
		}
		want := []string{filepath.Join(dir, "out_1.csv"), filepath.Join(dir, "out_2.csv")}
		if len(saved) != 2 || saved[0] != want[0] || saved[1] != want[1] {
			t.Fatalf("writeOutput() saved = %#v, want %#v", saved, want)
		}
		if got := readFile(t, want[0]); got != "id,name\n1,alice\n2,bob\n" {
			t.Fatalf("first file content = %q", got)
		}
		if got := readFile(t, want[1]); got != "total\n2\n" {
			t.Fatalf("second file content = %q", got)
		}
	})

	t.Run("count-only results have no rows to export", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "out.csv")
		_, err := writeOutput(path, "csv", []QueryResult{{RowsAffected: 3}})
		if err == nil {
			t.Fatal("writeOutput() expected error for result sets without columns")
		}
		if _, statErr := os.Stat(path); statErr == nil {
			t.Fatal("writeOutput() created a file despite having no rows")
		}
	})

	t.Run("unsupported format", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "out.xml")
		if _, err := writeOutput(path, "xml", []QueryResult{setA}); err == nil {
			t.Fatal("writeOutput() expected error for unsupported format")
		}
	})

	t.Run("nested output directory is created", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nested", "deeper", "out.json")
		if _, err := writeOutput(path, "json", []QueryResult{setA}); err != nil {
			t.Fatalf("writeOutput() error = %v", err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected file at %s: %v", path, err)
		}
	})
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	return string(readFileBytes(t, path))
}

func readFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return data
}
