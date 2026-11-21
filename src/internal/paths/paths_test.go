package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigPathOverride(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARPY_CONFIG", "~/custom/harpy.yaml")
	got, err := ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "custom", "harpy.yaml"); got != want {
		t.Fatalf("ConfigPath = %q, want %q", got, want)
	}
}

func TestDBPathOverride(t *testing.T) {
	t.Setenv("HARPY_DB", "C:/tmp/harpy.db")
	got, err := DBPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != "C:/tmp/harpy.db" {
		t.Fatalf("DBPath = %q", got)
	}
}

func TestLogsDirIsAbsolute(t *testing.T) {
	got, err := LogsDir()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("LogsDir is not absolute: %q", got)
	}
}
