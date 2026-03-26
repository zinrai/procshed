package main

import (
	"testing"
)

func TestParseStartTime(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    uint64
	}{
		{
			name:    "normal bash process",
			content: "1234 (bash) S 1233 1234 1234 0 -1 4194304 500 0 0 0 10 5 0 0 20 0 1 0 12345678 10000000 500 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0 0 0 0 0 0 0 0 0",
			want:    12345678,
		},
		{
			name:    "comm with spaces",
			content: "42 (tmux: server) S 1 42 42 0 -1 4194304 100 0 0 0 5 3 0 0 20 0 1 0 99999 5000000 200 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0 0 0 0 0 0 0 0 0",
			want:    99999,
		},
		{
			name:    "comm with parentheses",
			content: "100 (kworker/0:1-events (nice)) S 2 0 0 0 -1 69238880 0 0 0 0 0 0 0 0 20 0 1 0 55555 0 0 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0 0 0 0 0 0 0 0 0",
			want:    55555,
		},
		{
			name:    "large starttime",
			content: "1 (init) S 0 1 1 0 -1 4194304 1000 0 0 0 20 10 0 0 20 0 1 0 18446744073709551000 50000000 1000 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0 0 0 0 0 0 0 0 0",
			want:    18446744073709551000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseStartTime(tt.content)
			if err != nil {
				t.Fatalf("parseStartTime() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("parseStartTime() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseStartTimeErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "no closing parenthesis",
			content: "1234 (bash S 0 1 1",
		},
		{
			name:    "too few fields after comm",
			content: "1234 (bash) S 0 1 1",
		},
		{
			name:    "starttime not a number",
			content: "1234 (bash) S 1233 1234 1234 0 -1 4194304 500 0 0 0 10 5 0 0 20 0 1 0 notanumber 10000000 500 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0 0 0 0 0 0 0 0 0",
		},
		{
			name:    "empty string",
			content: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseStartTime(tt.content)
			if err == nil {
				t.Error("parseStartTime() expected error, got nil")
			}
		})
	}
}
