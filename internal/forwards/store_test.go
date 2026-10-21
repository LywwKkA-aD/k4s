package forwards

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func writeState(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "portforwards.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// entry is a stored forward as JSON with one field replaced.
func entry(field, value string) string {
	fields := map[string]string{
		"id": `"f1"`, "context": `"dev"`, "namespace": `"demo"`, "kind": `"pod"`,
		"name": `"web"`, "local_port": "18080", "remote_port": "80",
	}
	if value == "" {
		delete(fields, field)
	} else {
		fields[field] = value
	}
	parts := make([]string, 0, len(fields))
	for _, k := range []string{"id", "context", "server", "ca", "namespace", "kind", "name", "local_port", "remote_port"} {
		if v, ok := fields[k]; ok {
			parts = append(parts, `"`+k+`": `+v)
		}
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func TestLoadValidatesEveryEntry(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
		want string // "" when the file loads
	}{
		{name: "valid", body: `{"forwards": [` + entry("server", `"`+testServer+`"`) + `]}`},
		{name: "legacy without server", body: `{"forwards": [` + entry("", "") + `]}`},
		{name: "legacy without context", body: `{"forwards": [` + entry("context", "") + `]}`},
		{name: "not json", body: `{not json`, want: "parse"},
		{name: "server without context", body: `{"forwards": [` + strings.Replace(entry("context", ""), `"id"`, `"server": "`+testServer+`", "id"`, 1) + `]}`, want: "context"},
		{name: "dotted name", body: `{"forwards": [` + entry("name", `"a..b"`) + `]}`, want: `"a..b"`},
		{name: "dot-dot name", body: `{"forwards": [` + entry("name", `".."`) + `]}`, want: `".."`},
		{name: "path in name", body: `{"forwards": [` + entry("name", `"../../etc"`) + `]}`, want: "etc"},
		{name: "upper-case namespace", body: `{"forwards": [` + entry("namespace", `"Demo"`) + `]}`, want: "namespace"},
		{name: "unknown kind", body: `{"forwards": [` + entry("kind", `"job"`) + `]}`, want: "kind"},
		{name: "zero local port", body: `{"forwards": [` + entry("local_port", "0") + `]}`, want: "local port"},
		{name: "port over 65535", body: `{"forwards": [` + entry("remote_port", "70000") + `]}`, want: "parse"},
		{name: "negative port", body: `{"forwards": [` + entry("local_port", "-1") + `]}`, want: "parse"},
		{name: "empty id", body: `{"forwards": [` + entry("id", `""`) + `]}`, want: "id"},
		{name: "odd id", body: `{"forwards": [` + entry("id", `"a/b"`) + `]}`, want: "id"},
		{name: "control characters in context", body: `{"forwards": [` + entry("context", `"dev\u001b]0;x\u0007"`) + `]}`, want: "context"},
		{name: "malformed ca", body: `{"forwards": [` + entry("ca", `"md5:abc"`) + `]}`, want: "ca"},
		{name: "duplicate ids", body: `{"forwards": [` + entry("", "") + `, ` + entry("", "") + `]}`, want: "duplicate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := loadFrom(writeState(t, tc.body))
			switch {
			case tc.want == "" && err != nil:
				t.Errorf("loadFrom: %v", err)
			case tc.want != "" && err == nil:
				t.Errorf("loadFrom accepted %s", tc.body)
			case tc.want != "" && !strings.Contains(err.Error(), tc.want):
				t.Errorf("loadFrom error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestValidateRejectsNamesKubernetesWould(t *testing.T) {
	t.Parallel()
	base := testForward("f1", "pod", 1)
	cases := []struct {
		name string
		edit func(*Forward)
	}{
		{name: "dot-dot in name", edit: func(f *Forward) { f.Name = "web..1" }},
		{name: "slash in name", edit: func(f *Forward) { f.Name = "../web" }},
		{name: "namespace with dots", edit: func(f *Forward) { f.Namespace = "a.b" }},
		{name: "id with a slash", edit: func(f *Forward) { f.ID = "../f1" }},
		{name: "context with an escape", edit: func(f *Forward) { f.Context = "dev\x1b[2J" }},
		{name: "server with a newline", edit: func(f *Forward) { f.Server = testServer + "\n" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := base
			tc.edit(&f)
			if err := f.Validate(); err == nil {
				t.Errorf("Validate accepted %+v", f)
			}
		})
	}
}

func TestNewManagerSetsACorruptFileAside(t *testing.T) {
	t.Parallel()
	path := writeState(t, `{"forwards": [`+entry("name", `"a..b"`)+`]}`)
	m, err := newManager(deps{statePath: path, cluster: thisCluster})
	if err != nil {
		t.Fatalf("newManager: %v", err)
	}
	if len(m.List()) != 0 {
		t.Errorf("List = %+v, want empty after a corrupt file", m.List())
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the corrupt file is still at %s: %v", path, err)
	}
	aside, err := filepath.Glob(path + ".corrupt-*")
	if err != nil || len(aside) != 1 {
		t.Fatalf("set aside = %v (%v), want one .corrupt- file", aside, err)
	}
	notice := m.TakeNotice()
	if !strings.Contains(notice, filepath.Base(aside[0])) || !strings.Contains(notice, "a..b") {
		t.Errorf("notice = %q, want the new name and the reason", notice)
	}
	if again := m.TakeNotice(); again != "" {
		t.Errorf("second TakeNotice = %q, want it said once", again)
	}

	// The next save starts a fresh file; the corrupt one stays for the user.
	mustRegister(t, m, testForward("f2", "pod", 18081))
	if _, err := os.Stat(aside[0]); err != nil {
		t.Errorf("the set-aside file went away: %v", err)
	}
}

func TestNewManagerFailsOnAnUnreadableState(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "portforwards.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := newManager(deps{statePath: path}); err == nil {
		t.Fatal("newManager accepted a state path it cannot read")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("an unreadable state was moved: %v", err)
	}
}

func TestCorruptName(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 21, 9, 5, 7, 123456789, time.FixedZone("CEST", 2*3600))
	if got := corruptName("/s/portforwards.json", at); got != "/s/portforwards.json.corrupt-20261021T070507.123Z" {
		t.Errorf("corruptName = %q", got)
	}
}

func TestSaveRestrictsTheStateDir(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX permission bits")
	}
	dir := filepath.Join(t.TempDir(), "k4s")
	if err := os.Mkdir(dir, 0o755); err != nil { //nolint:gosec // the test starts from a too open directory
		t.Fatal(err)
	}
	if err := (State{}).saveTo(filepath.Join(dir, "portforwards.json")); err != nil {
		t.Fatalf("saveTo: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("state dir mode = %o, want 700", got)
	}
}

// TestStatePathIgnoresARelativeXDGStateHome cannot run in parallel: it
// sets XDG_STATE_HOME.
func TestStatePathIgnoresARelativeXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "relative/state")
	path, err := statePath()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(path) || !strings.HasSuffix(path, filepath.Join(".local", "state", "k4s", "portforwards.json")) {
		t.Errorf("statePath = %q, want the home fallback", path)
	}
}

func TestRegisterSavesBeforeItChangesMemory(t *testing.T) {
	t.Parallel()
	dir := lockableDir(t)
	path := filepath.Join(dir, "portforwards.json")
	m := newTestManager(t, deps{statePath: path})
	f := testForward("f1", "pod", 18080)
	mustRegister(t, m, f)
	setWritable(t, dir, false)

	changed := m.Changes()
	moved := testForward("f1", "pod", 18081)
	assertErrContains(t, m.Register(moved), "save state")
	assertErrContains(t, m.Register(testForward("f2", "pod", 18082)), "save state")
	if a := statusOf(t, m, "f1"); a.Forward != f {
		t.Errorf("after a failed save f1 is %+v, want %+v", a.Forward, f)
	}
	if _, ok := find(m.List(), "f2"); ok {
		t.Error("a forward whose save failed is listed")
	}
	if got := m.State().Forwards; len(got) != 1 || got[0] != f {
		t.Errorf("State = %+v, want only %+v", got, f)
	}
	select {
	case <-changed:
		t.Error("a failed Register signalled a change")
	default:
	}
}

func TestRegisterRefusesAnIDWithALiveSession(t *testing.T) {
	t.Parallel()
	fake := newFakeTunneler(t)
	m := newTestManager(t, deps{tunnel: fake})
	f := testForward("f1", "pod", 18080)
	mustRegister(t, m, f)
	running(t, m, fake, "f1")

	moved := testForward("f1", "pod", 18081)
	if err := m.Register(moved); !errors.Is(err, ErrRunning) {
		t.Errorf("Register over a running forward = %v, want ErrRunning", err)
	}
	if a := statusOf(t, m, "f1"); a.Forward != f || a.Status != StatusRunning {
		t.Errorf("f1 = %+v, want it running unchanged", a)
	}
	if err := m.Stop("f1"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	mustRegister(t, m, moved)
}
