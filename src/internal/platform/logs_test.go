package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogStoreOpenRunSanitizesNames(t *testing.T) {
	store := NewLogStore()
	dir := t.TempDir()
	run := store.OpenRun(dir)
	if run.Dir() != dir {
		t.Fatalf("Dir = %q", run.Dir())
	}

	file, err := run.Create("apt/weird:name")
	if err != nil {
		t.Fatal(err)
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(name) != "apt_weird_name.log" {
		t.Fatalf("log name = %q", name)
	}
	if _, err := os.Stat(name); err != nil {
		t.Fatal(err)
	}
}

func TestLogStoreNewRunUsesStateDir(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_STATE_HOME", base)
	t.Setenv("LOCALAPPDATA", base)

	run, err := NewLogStore().NewRun()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Clean(run.Dir()), filepath.Clean(filepath.Join(base, "harpy", "logs"))) {
		t.Fatalf("run dir = %q", run.Dir())
	}
}
