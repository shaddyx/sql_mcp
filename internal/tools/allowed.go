package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// allowedDirsEnv lists where output files may be created: a comma-separated
// list of glob patterns, e.g. "/tmp/**,{cwd}/**".
const allowedDirsEnv = "ALLOWED_DIRS"

// defaultAllowedDirs applies when the environment variable is unset: exports
// are restricted to the working directory tree.
const defaultAllowedDirs = "{cwd}/**"

// cwdToken is replaced with the process working directory in patterns.
const cwdToken = "{cwd}"

// pathGuard decides whether an absolute path may be written to, based on the
// ALLOWED_DIRS patterns. In patterns, ** matches any number of path
// segments, * matches within one segment, and ? matches a single character.
// A pattern without glob characters names a directory and allows everything
// beneath it.
type pathGuard struct {
	patterns []string
	regexes  []*regexp.Regexp
	prefixes []string
}

// outputGuard builds the guard from the environment.
func outputGuard() *pathGuard {
	raw, ok := os.LookupEnv(allowedDirsEnv)
	if !ok {
		raw = defaultAllowedDirs
	}
	return newGuard(raw)
}

func newGuard(raw string) *pathGuard {
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	g := &pathGuard{}
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		g.patterns = append(g.patterns, p)
		p = strings.ReplaceAll(p, cwdToken, wd)
		if !strings.ContainsAny(p, "*?") {
			g.prefixes = append(g.prefixes, filepath.Clean(p))
			continue
		}
		if re, err := globToRegexp(p); err == nil {
			g.regexes = append(g.regexes, re)
		}
	}
	return g
}

// allow reports whether absPath matches at least one pattern.
func (g *pathGuard) allow(absPath string) bool {
	for _, re := range g.regexes {
		if re.MatchString(absPath) {
			return true
		}
	}
	for _, p := range g.prefixes {
		if absPath == p || strings.HasPrefix(absPath, withTrailingSep(p)) {
			return true
		}
	}
	return false
}

// checkWritable resolves path against the working directory and returns an
// error unless the guard allows writing to it.
func (g *pathGuard) checkWritable(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolving output_path: %w", err)
	}
	abs = filepath.Clean(abs)
	if !g.allow(abs) {
		return fmt.Errorf("output_path %q is outside %s (allowed patterns: %s)",
			path, allowedDirsEnv, strings.Join(g.patterns, ", "))
	}
	return nil
}

func withTrailingSep(p string) string {
	if strings.HasSuffix(p, string(filepath.Separator)) {
		return p
	}
	return p + string(filepath.Separator)
}

// globToRegexp translates a slash-separated glob into an anchored regexp:
// ** spans path separators, * and ? stay within one segment.
func globToRegexp(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i++
				for i+1 < len(pattern) && pattern[i+1] == '*' {
					i++
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}
