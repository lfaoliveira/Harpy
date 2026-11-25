package platform

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestToSlash(t *testing.T) {
	if runtime.GOOS == "windows" {
		if got := ToSlash(`C:\Users\me\harpy`); got != "C:/Users/me/harpy" {
			t.Fatalf("ToSlash = %q", got)
		}
		return
	}
	if got := ToSlash("/home/me/harpy"); got != "/home/me/harpy" {
		t.Fatalf("ToSlash = %q", got)
	}
}
