package kubectl_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
)

func TestCheckLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in string
		ok       bool
	}{
		{"namespace", "k4s-demo", true},
		{"digits first", "0abc", true},
		{"max length", strings.Repeat("a", 63), true},
		{"too long", strings.Repeat("a", 64), false},
		{"empty", "", false},
		{"leading dash reads as a flag", "-n", false},
		{"upper case", "Demo", false},
		{"dot", "a.b", false},
		{"space", "a b", false},
		{"shell metacharacter", "a;b", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := kubectl.CheckLabel("namespace", tt.in)
			if (err == nil) != tt.ok {
				t.Fatalf("CheckLabel(%q) = %v, want ok=%v", tt.in, err, tt.ok)
			}
			if err != nil && !errors.Is(err, kubectl.ErrInvalidName) {
				t.Errorf("error %v does not wrap ErrInvalidName", err)
			}
		})
	}
}

func TestCheckSubdomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in string
		ok       bool
	}{
		{"pod", "growlytics-app-7d9f8c6b5d-abcde", true},
		{"dots allowed", "web.v2", true},
		{"max length", strings.Repeat("a", 253), true},
		{"too long", strings.Repeat("a", 254), false},
		{"empty", "", false},
		{"leading dash", "--previous", false},
		{"upper case", "Web", false},
		{"slash", "deploy/web", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := kubectl.CheckSubdomain("pod", tt.in)
			if (err == nil) != tt.ok {
				t.Fatalf("CheckSubdomain(%q) = %v, want ok=%v", tt.in, err, tt.ok)
			}
			if err != nil && !errors.Is(err, kubectl.ErrInvalidName) {
				t.Errorf("error %v does not wrap ErrInvalidName", err)
			}
		})
	}
}

func TestCheckNameErrorQuotesTheName(t *testing.T) {
	t.Parallel()

	err := kubectl.CheckLabel("container", "evil\x1b[2J")
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.ContainsRune(err.Error(), '\x1b') {
		t.Errorf("error carries a raw escape: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "container") {
		t.Errorf("error does not name the kind: %q", err.Error())
	}
}
