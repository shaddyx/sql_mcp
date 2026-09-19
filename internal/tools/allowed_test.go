package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewGuardMatching(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}

	tests := []struct {
		name    string
		raw     string
		path    string
		allowed bool
	}{
		{
			name:    "doublestar matches nested file",
			raw:     "/tmp/**",
			path:    "/tmp/a/b/out.csv",
			allowed: true,
		},
		{
			name:    "doublestar matches direct child",
			raw:     "/tmp/**",
			path:    "/tmp/out.csv",
			allowed: true,
		},
		{
			name:    "doublestar rejects outside prefix",
			raw:     "/tmp/**",
			path:    "/etc/passwd",
			allowed: false,
		},
		{
			name:    "cwd token expands to working directory",
			raw:     "{cwd}/**",
			path:    filepath.Join(wd, "export", "out.json"),
			allowed: true,
		},
		{
			name:    "cwd token does not match elsewhere",
			raw:     "{cwd}/**",
			path:    "/somewhere/else/out.json",
			allowed: false,
		},
		{
			name:    "comma separated list",
			raw:     "/tmp/**,{cwd}/**",
			path:    filepath.Join(wd, "out.csv"),
			allowed: true,
		},
		{
			name:    "single star does not cross separator",
			raw:     "/data/*/out.csv",
			path:    "/data/x/out.csv",
			allowed: true,
		},
		{
			name:    "single star rejects nested path",
			raw:     "/data/*/out.csv",
			path:    "/data/x/y/out.csv",
			allowed: false,
		},
		{
			name:    "extension glob",
			raw:     "/tmp/*.csv",
			path:    "/tmp/a.csv",
			allowed: true,
		},
		{
			name:    "extension glob rejects other extension",
			raw:     "/tmp/*.csv",
			path:    "/tmp/a.json",
			allowed: false,
		},
		{
			name:    "question mark matches single character",
			raw:     "/data/?/out.csv",
			path:    "/data/a/out.csv",
			allowed: true,
		},
		{
			name:    "question mark rejects longer segment",
			raw:     "/data/?/out.csv",
			path:    "/data/ab/out.csv",
			allowed: false,
		},
		{
			name:    "plain directory pattern allows everything below",
			raw:     "/data/reports",
			path:    "/data/reports/2026/out.csv",
			allowed: true,
		},
		{
			name:    "plain directory pattern matches the directory itself",
			raw:     "/data/reports",
			path:    "/data/reports",
			allowed: true,
		},
		{
			name:    "plain directory pattern rejects sibling prefix",
			raw:     "/data/reports",
			path:    "/data/reports-archive/out.csv",
			allowed: false,
		},
		{
			name:    "empty value allows nothing",
			raw:     "",
			path:    "/tmp/out.csv",
			allowed: false,
		},
		{
			name:    "dots in pattern are literal",
			raw:     "/tmp/out.csv",
			path:    "/tmp/XoutXcsv",
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newGuard(tt.raw)
			if got := g.allow(tt.path); got != tt.allowed {
				t.Fatalf("newGuard(%q).allow(%q) = %v, want %v", tt.raw, tt.path, got, tt.allowed)
			}
		})
	}
}

func TestGuardCheckWritable(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}

	g := newGuard("{cwd}/**")

	if err := g.checkWritable("relative/out.csv"); err != nil {
		t.Fatalf("checkWritable(relative) error = %v, want nil", err)
	}
	if err := g.checkWritable(filepath.Join(wd, "nested", "out.json")); err != nil {
		t.Fatalf("checkWritable(cwd child) error = %v, want nil", err)
	}
	err = g.checkWritable("/etc/out.csv")
	if err == nil {
		t.Fatal("checkWritable(/etc) error = nil, want error")
	}
	if !strings.Contains(err.Error(), allowedDirsEnv) {
		t.Fatalf("checkWritable(/etc) error = %v, want mention of %s", err, allowedDirsEnv)
	}
}

func TestOutputGuardDefault(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	t.Setenv(allowedDirsEnv, "")
	// An explicitly set empty value restricts everything.
	if outputGuard().allow(filepath.Join(wd, "out.csv")) {
		t.Fatal("explicitly empty ALLOWED_DIRS should allow nothing")
	}
	os.Unsetenv(allowedDirsEnv)
	if !outputGuard().allow(filepath.Join(wd, "out.csv")) {
		t.Fatal("unset ALLOWED_DIRS should default to the working directory tree")
	}
	if outputGuard().allow("/tmp/out.csv") {
		t.Fatal("default guard should reject paths outside the working directory")
	}
}
