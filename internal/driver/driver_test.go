package driver

import (
	"net/url"
	"path/filepath"
	"testing"
)

func TestOpenPostgres(t *testing.T) {
	db, err := Open("postgresql://user:pass@localhost:5432/mydb")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if db == nil {
		t.Fatal("Open() returned nil db")
	}
}

func TestOpenMySQL(t *testing.T) {
	db, err := Open("mysql://user:pass@localhost:3306/mydb")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if db == nil {
		t.Fatal("Open() returned nil db")
	}
}

func TestOpenSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open("sqlite://" + path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatalf("db.Ping() error = %v", err)
	}

	var one int
	if err := db.QueryRow("SELECT 1").Scan(&one); err != nil {
		t.Fatalf("QueryRow error = %v", err)
	}
	if one != 1 {
		t.Fatalf("SELECT 1 = %d, want 1", one)
	}
}

func TestOpenPlainSQLitePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plain.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
}

func TestOpenUnsupported(t *testing.T) {
	_, err := Open("oracle://user:pass@host/db")
	if err == nil {
		t.Fatal("Open() expected error for unsupported scheme")
	}
	want := `unsupported connection url: "oracle://user:pass@host/db"`
	if err.Error() != want {
		t.Errorf("Open() error = %q, want %q", err.Error(), want)
	}
}

func TestOpenSQLiteFilePrefix(t *testing.T) {
	// sql.Open is lazy, so a nonexistent path still succeeds and pins the
	// "file:" prefix branch of the SQLite matcher.
	db, err := Open("file:/tmp/nonexistent-review.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if db == nil {
		t.Fatal("Open() returned nil db")
	}
}

func TestSupportedList(t *testing.T) {
	want := "PostgreSQL (postgres:// / postgresql://), MySQL (mysql://), " +
		"SQLite (sqlite:// / file: / plain file path)"
	got := SupportedList()
	if got != want {
		t.Errorf("SupportedList() = %q, want %q", got, want)
	}
}

func TestSupported(t *testing.T) {
	got := Supported()
	if len(got) != 3 {
		t.Fatalf("Supported() returned %d engines, want 3", len(got))
	}

	wantNames := []string{"PostgreSQL", "MySQL", "SQLite"}
	for i, name := range wantNames {
		if got[i].Name != name {
			t.Errorf("Supported()[%d].Name = %q, want %q", i, got[i].Name, name)
		}
	}

	// Mutating the returned slice must not affect subsequent calls.
	got[0].Name = "Mutated"
	got[0].Schemes[0] = "mutated://"
	fresh := Supported()
	if fresh[0].Name != "PostgreSQL" {
		t.Errorf("Supported()[0].Name = %q after mutation, want %q", fresh[0].Name, "PostgreSQL")
	}
	if fresh[0].Schemes[0] != "postgres://" {
		t.Errorf("Supported()[0].Schemes[0] = %q after mutation, want %q", fresh[0].Schemes[0], "postgres://")
	}
}

func TestMySQLDSN(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "user and password",
			raw:  "mysql://user:pass@localhost:3306/mydb",
			want: "user:pass@tcp(localhost:3306)/mydb?charset=utf8mb4&parseTime=true",
		},
		{
			name: "no credentials",
			raw:  "mysql://localhost/mydb",
			want: "tcp(localhost)/mydb?charset=utf8mb4&parseTime=true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.raw)
			if err != nil {
				t.Fatalf("url.Parse() error = %v", err)
			}
			got, err := mysqlDSN(u)
			if err != nil {
				t.Fatalf("mysqlDSN() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("mysqlDSN() = %q, want %q", got, tt.want)
			}
		})
	}
}
