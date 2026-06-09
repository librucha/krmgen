package cmd

import (
	"testing"
)

func TestMatchesSkipPattern(t *testing.T) {
	tests := []struct {
		name     string
		relPath  string
		patterns []string
		want     bool
	}{
		{
			name:     "no patterns",
			relPath:  "cert.pfx",
			patterns: nil,
			want:     false,
		},
		{
			name:     "extension wildcard matches flat file",
			relPath:  "cert.pfx",
			patterns: []string{"*.pfx"},
			want:     true,
		},
		{
			name:     "extension wildcard matches nested file by basename",
			relPath:  "certs/prod/cert.pfx",
			patterns: []string{"*.pfx"},
			want:     true,
		},
		{
			name:     "extension wildcard does not match different extension",
			relPath:  "cert.pem",
			patterns: []string{"*.pfx"},
			want:     false,
		},
		{
			name:     "directory-scoped pattern matches file in that directory",
			relPath:  "assets/logo.png",
			patterns: []string{"assets/*.png"},
			want:     true,
		},
		{
			name:     "directory-scoped pattern does not match file in other directory",
			relPath:  "static/logo.png",
			patterns: []string{"assets/*.png"},
			want:     false,
		},
		{
			name:     "exact filename match",
			relPath:  "secret.yaml",
			patterns: []string{"secret.yaml"},
			want:     true,
		},
		{
			name:     "exact filename does not match different file",
			relPath:  "config.yaml",
			patterns: []string{"secret.yaml"},
			want:     false,
		},
		{
			name:     "first matching pattern wins",
			relPath:  "cert.pfx",
			patterns: []string{"*.pem", "*.pfx", "*.crt"},
			want:     true,
		},
		{
			name:     "multiple patterns, none match",
			relPath:  "values.yaml",
			patterns: []string{"*.pfx", "*.png", "*.pem"},
			want:     false,
		},
		{
			name:     "question mark wildcard matches single char",
			relPath:  "cert.p12",
			patterns: []string{"cert.p??"},
			want:     true,
		},
		{
			name:     "empty relPath with matching pattern",
			relPath:  "file.bin",
			patterns: []string{"*.bin"},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesSkipPattern(tt.relPath, tt.patterns)
			if got != tt.want {
				t.Errorf("matchesSkipPattern(%q, %v) = %v, want %v", tt.relPath, tt.patterns, got, tt.want)
			}
		})
	}
}

func TestMergeSkipPatterns(t *testing.T) {
	tests := []struct {
		name string
		a    []string
		b    []string
		want []string
	}{
		{
			name: "both empty",
			a:    nil,
			b:    nil,
			want: nil,
		},
		{
			name: "only a",
			a:    []string{"*.pfx"},
			b:    nil,
			want: []string{"*.pfx"},
		},
		{
			name: "only b",
			a:    nil,
			b:    []string{"*.png"},
			want: []string{"*.png"},
		},
		{
			name: "no duplicates",
			a:    []string{"*.pfx", "*.png"},
			b:    []string{"*.pem", "*.bin"},
			want: []string{"*.pfx", "*.png", "*.pem", "*.bin"},
		},
		{
			name: "deduplicates across a and b",
			a:    []string{"*.pfx", "*.png"},
			b:    []string{"*.png", "*.pem"},
			want: []string{"*.pfx", "*.png", "*.pem"},
		},
		{
			name: "deduplicates within a",
			a:    []string{"*.pfx", "*.pfx"},
			b:    nil,
			want: []string{"*.pfx"},
		},
		{
			name: "config patterns come before cli patterns",
			a:    []string{"*.pfx"},
			b:    []string{"*.png"},
			want: []string{"*.pfx", "*.png"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeSkipPatterns(tt.a, tt.b)
			if len(got) != len(tt.want) {
				t.Fatalf("mergeSkipPatterns(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("mergeSkipPatterns result[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
