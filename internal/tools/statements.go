package tools

import (
	"strconv"
	"strings"
)

// splitStatements splits a SQL batch into individual statements on
// top-level semicolons, ignoring semicolons inside string literals,
// quoted identifiers, and comments. Whitespace-only statements are
// dropped.
func splitStatements(query string) []string {
	var stmts []string
	var cur strings.Builder
	var quote byte
	inLine, inBlock := false, false

	b := []byte(query)
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case inLine:
			cur.WriteByte(c)
			if c == '\n' {
				inLine = false
			}
		case inBlock:
			cur.WriteByte(c)
			if c == '*' && i+1 < len(b) && b[i+1] == '/' {
				cur.WriteByte('/')
				i++
				inBlock = false
			}
		case quote != 0:
			cur.WriteByte(c)
			if c == quote {
				if i+1 < len(b) && b[i+1] == quote {
					cur.WriteByte(quote)
					i++
				} else {
					quote = 0
				}
			}
		default:
			switch {
			case c == '\'' || c == '"' || c == '`':
				quote = c
				cur.WriteByte(c)
			case c == '-' && i+1 < len(b) && b[i+1] == '-':
				inLine = true
				cur.WriteByte(c)
			case c == '/' && i+1 < len(b) && b[i+1] == '*':
				inBlock = true
				cur.WriteByte(c)
			case c == ';':
				if s := strings.TrimSpace(cur.String()); s != "" {
					stmts = append(stmts, s)
				}
				cur.Reset()
			default:
				cur.WriteByte(c)
			}
		}
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		stmts = append(stmts, s)
	}
	return stmts
}

// placeholderCount returns how many positional parameters a statement
// expects: the highest $N for Postgres-style placeholders, otherwise the
// number of ? markers. Placeholders inside literals and comments are
// ignored. Statements with no placeholders return 0.
func placeholderCount(stmt string) int {
	b := []byte(stmt)
	var quote byte
	inLine, inBlock := false, false

	positional := 0
	dollarMax := 0
	count := 0

	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case inLine:
			if c == '\n' {
				inLine = false
			}
		case inBlock:
			if c == '*' && i+1 < len(b) && b[i+1] == '/' {
				i++
				inBlock = false
			}
		case quote != 0:
			if c == quote {
				if i+1 < len(b) && b[i+1] == quote {
					i++
				} else {
					quote = 0
				}
			}
		default:
			switch {
			case c == '\'' || c == '"' || c == '`':
				quote = c
			case c == '-' && i+1 < len(b) && b[i+1] == '-':
				inLine = true
			case c == '/' && i+1 < len(b) && b[i+1] == '*':
				inBlock = true
			case c == '?':
				count++
			case c == '$' && i+1 < len(b) && b[i+1] >= '0' && b[i+1] <= '9':
				dollarMax, i = parseDollar(b, i)
				positional++
			}
		}
	}

	if positional > 0 {
		return dollarMax
	}
	return count
}

// parseDollar reads a $N parameter reference starting at b[i] == '$' and
// returns the parsed number and the index of the last consumed byte.
func parseDollar(b []byte, i int) (int, int) {
	j := i + 1
	for j < len(b) && b[j] >= '0' && b[j] <= '9' {
		j++
	}
	n, err := strconv.Atoi(string(b[i+1 : j]))
	if err != nil {
		return 0, i
	}
	return n, j - 1
}
