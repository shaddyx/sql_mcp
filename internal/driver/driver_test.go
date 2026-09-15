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
	if _, err := Open("oracle://user:pass@host/db"); err == nil {
		t.Fatal("Open() expected error for unsupported scheme")
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
