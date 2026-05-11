package forwards

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRejectsEmptyFields(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		f    Forward
		want string
	}{
		{"empty id", Forward{Namespace: "n", Kind: "pod", Name: "x", LocalPort: 1, RemotePort: 1}, "id"},
		{"empty namespace", Forward{ID: "a", Kind: "pod", Name: "x", LocalPort: 1, RemotePort: 1}, "namespace"},
		{"empty name", Forward{ID: "a", Namespace: "n", Kind: "pod", LocalPort: 1, RemotePort: 1}, "name"},
		{"bad kind", Forward{ID: "a", Namespace: "n", Kind: "junk", Name: "x", LocalPort: 1, RemotePort: 1}, "kind"},
		{"zero local port", Forward{ID: "a", Namespace: "n", Kind: "pod", Name: "x", RemotePort: 1}, "local port"},
		{"zero remote port", Forward{ID: "a", Namespace: "n", Kind: "pod", Name: "x", LocalPort: 1}, "remote port"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.f.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not mention %q", err.Error(), tt.want)
			}
		})
	}
}

func TestValidateAcceptsAllKnownKinds(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"pod", "service", "deployment"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := Forward{ID: "a", Namespace: "n", Kind: kind, Name: "x", LocalPort: 8080, RemotePort: 80}
			if err := f.Validate(); err != nil {
				t.Errorf("kind=%s should validate, got %v", kind, err)
			}
		})
	}
}

func TestFindByIDHitsAndMisses(t *testing.T) {
	t.Parallel()
	s := State{Forwards: []Forward{
		{ID: "a", Name: "alpha"},
		{ID: "b", Name: "beta"},
	}}
	got, ok := s.FindByID("b")
	if !ok || got.Name != "beta" {
		t.Errorf("FindByID(b) = %+v / ok=%v, want beta / true", got, ok)
	}
	_, ok = s.FindByID("missing")
	if ok {
		t.Errorf("FindByID(missing) should miss")
	}
}

func TestUpsertReplacesExistingByID(t *testing.T) {
	t.Parallel()
	s := State{Forwards: []Forward{
		{ID: "a", LocalPort: 8080},
		{ID: "b", LocalPort: 9090},
	}}
	updated := s.Upsert(Forward{ID: "a", LocalPort: 7000})
	if len(updated.Forwards) != 2 {
		t.Errorf("upsert-replace should keep length=2, got %d", len(updated.Forwards))
	}
	got, _ := updated.FindByID("a")
	if got.LocalPort != 7000 {
		t.Errorf("upsert did not replace, got %d", got.LocalPort)
	}
}

func TestUpsertAppendsNew(t *testing.T) {
	t.Parallel()
	s := State{Forwards: []Forward{{ID: "a"}}}
	updated := s.Upsert(Forward{ID: "b"})
	if len(updated.Forwards) != 2 {
		t.Errorf("upsert-append should grow to 2, got %d", len(updated.Forwards))
	}
}

func TestUpsertDoesNotMutateOriginal(t *testing.T) {
	t.Parallel()
	// Immutability: callers reading the State after an Upsert must see
	// the *old* one until they replace it themselves.
	s := State{Forwards: []Forward{{ID: "a", LocalPort: 8080}}}
	_ = s.Upsert(Forward{ID: "a", LocalPort: 9999})
	if s.Forwards[0].LocalPort != 8080 {
		t.Errorf("Upsert mutated the receiver: %+v", s.Forwards[0])
	}
}

func TestRemoveDropsByID(t *testing.T) {
	t.Parallel()
	s := State{Forwards: []Forward{{ID: "a"}, {ID: "b"}, {ID: "c"}}}
	updated := s.Remove("b")
	if len(updated.Forwards) != 2 {
		t.Errorf("after Remove length=%d, want 2", len(updated.Forwards))
	}
	if _, ok := updated.FindByID("b"); ok {
		t.Errorf("Remove failed: b still present")
	}
}

func TestRemoveMissingIsNoOp(t *testing.T) {
	t.Parallel()
	s := State{Forwards: []Forward{{ID: "a"}}}
	updated := s.Remove("ghost")
	if len(updated.Forwards) != 1 {
		t.Errorf("Remove missing id should be a no-op, got length=%d", len(updated.Forwards))
	}
}

func TestLoadFromMissingFileReturnsEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "absent.json")
	s, err := loadFrom(path)
	if err != nil {
		t.Errorf("missing file should not error, got %v", err)
	}
	if len(s.Forwards) != 0 {
		t.Errorf("missing file should yield empty state, got %+v", s)
	}
}

func TestLoadFromEmptyFileReturnsEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := loadFrom(path)
	if err != nil {
		t.Errorf("empty file should not error, got %v", err)
	}
	if len(s.Forwards) != 0 {
		t.Errorf("empty file should yield empty state")
	}
}

func TestLoadFromMalformedReportsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := loadFrom(path)
	if err == nil {
		t.Errorf("expected parse error on malformed JSON")
	}
}

func TestSaveAndLoadRoundtrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "round.json")
	s := State{Forwards: []Forward{
		{ID: "a", Context: "default", Namespace: "demo", Kind: "service", Name: "nginx", LocalPort: 8080, RemotePort: 80},
		{ID: "b", Context: "default", Namespace: "demo", Kind: "pod", Name: "alpha-xxx", LocalPort: 5432, RemotePort: 5432},
	}}
	if err := s.saveTo(path); err != nil {
		t.Fatalf("saveTo: %v", err)
	}
	got, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if len(got.Forwards) != 2 {
		t.Fatalf("roundtrip lost entries: got %d, want 2", len(got.Forwards))
	}
	if got.Forwards[0] != s.Forwards[0] {
		t.Errorf("first entry differs: %+v vs %+v", got.Forwards[0], s.Forwards[0])
	}
}

func TestSaveAtomicLeavesNoTemp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "atomic.json")
	s := State{Forwards: []Forward{{ID: "a", Namespace: "n", Kind: "pod", Name: "x", LocalPort: 1, RemotePort: 1}}}
	if err := s.saveTo(path); err != nil {
		t.Fatalf("saveTo: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("temp file leaked: %s", e.Name())
		}
	}
}

func TestSaveProducesValidJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "valid.json")
	s := State{Forwards: []Forward{{ID: "a", Namespace: "n", Kind: "pod", Name: "x", LocalPort: 8080, RemotePort: 80}}}
	if err := s.saveTo(path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var sanity State
	if err := json.Unmarshal(raw, &sanity); err != nil {
		t.Errorf("saved file is not valid JSON: %v\n%s", err, raw)
	}
	if len(sanity.Forwards) != 1 {
		t.Errorf("saved file lost the forward: %+v", sanity)
	}
}

func TestStatePathHonoursXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/k4s-xdg-test")
	path, err := statePath()
	if err != nil {
		t.Fatal(err)
	}
	want := "/tmp/k4s-xdg-test/k4s/portforwards.json"
	if path != want {
		t.Errorf("statePath() = %q, want %q", path, want)
	}
}

func TestStatePathFallsBackToHomeStateDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	path, err := statePath()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, filepath.Join(".local", "state", "k4s", "portforwards.json")) {
		t.Errorf("fallback path = %q, expected to end with .local/state/k4s/portforwards.json", path)
	}
}
