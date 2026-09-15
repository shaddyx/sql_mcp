package driver

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// Open parses a connection url and opens a *sql.DB for the matching engine.
// Supported schemes: postgres/postgresql, mysql, sqlite (a plain file path is
// also accepted and treated as a SQLite database).
func Open(rawURL string) (*sql.DB, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid connection url: %w", err)
	}

	switch {
	case u.Scheme == "postgres" || u.Scheme == "postgresql":
		dsn, err := postgresDSN(u)
		if err != nil {
			return nil, err
		}
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			return nil, fmt.Errorf("opening postgres connection: %w", err)
		}
		return db, nil

	case u.Scheme == "mysql":
		dsn, err := mysqlDSN(u)
		if err != nil {
			return nil, err
		}
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			return nil, fmt.Errorf("opening mysql connection: %w", err)
		}
		return db, nil

	case u.Scheme == "sqlite":
		db, err := sql.Open("sqlite", u.Opaque)
		if err != nil {
			return nil, fmt.Errorf("opening sqlite connection: %w", err)
		}
		return db, nil

	case strings.HasPrefix(rawURL, "file:"):
		path := strings.TrimPrefix(rawURL, "file:")
		db, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, fmt.Errorf("opening sqlite connection: %w", err)
		}
		return db, nil

	case !u.IsAbs() && isPlainPath(rawURL):
		db, err := sql.Open("sqlite", rawURL)
		if err != nil {
			return nil, fmt.Errorf("opening sqlite connection: %w", err)
		}
		return db, nil

	default:
		return nil, fmt.Errorf("unsupported connection url: %q", rawURL)
	}
}

// isPlainPath reports whether the raw string has no scheme component, i.e. it
// can only be a filesystem path to a SQLite database.
func isPlainPath(raw string) bool {
	return !strings.Contains(raw, "://") && !strings.Contains(raw, ":")
}

// postgresDSN converts a postgres url into a lib/pq DSN. In the common
// local/dev case the server has no SSL, so we default to "sslmode=disable"
// unless the caller explicitly set an sslmode query parameter.
func postgresDSN(u *url.URL) (string, error) {
	q := u.Query()
	if !q.Has("sslmode") {
		q.Set("sslmode", "disable")
	}
	u2 := *u
	u2.RawQuery = q.Encode()
	return u2.String(), nil
}

// mysqlDSN converts a mysql:// url into the go-sql-driver DSN format.
// Example: mysql://user:pass@host:3306/dbname?parseTime=true
// -> user:pass@tcp(host:3306)/dbname?parseTime=true
func mysqlDSN(u *url.URL) (string, error) {
	user := u.User
	host := u.Host
	if host == "" {
		return "", fmt.Errorf("mysql connection url missing host")
	}

	var b strings.Builder
	if user != nil {
		b.WriteString(user.String())
		b.WriteString("@")
	}
	b.WriteString("tcp(")
	b.WriteString(host)
	b.WriteString(")")

	path := strings.TrimPrefix(u.Path, "/")
	if path != "" {
		b.WriteString("/")
		b.WriteString(path)
	}

	q := u.Query()
	q.Set("parseTime", "true")
	if _, ok := q["charset"]; !ok {
		q.Set("charset", "utf8mb4")
	}
	if enc := q.Encode(); enc != "" {
		b.WriteString("?")
		b.WriteString(enc)
	}

	return b.String(), nil
}
