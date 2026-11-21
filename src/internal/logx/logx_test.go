package logx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeName(t *testing.T) {
	tests := map[string]string{
		"apt-git":           "apt-git",
		"apt/ripgrep:amd64": "apt_ripgrep_amd64",
		"weird name":        "weird_name",
		"..":                "package",
		"pkg@1.0":           "pkg_1.0",
	}
	for in, want := range tests {
		if got := SafeName(in); got != want {
			t.Errorf("SafeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRunCreatesPackageLogs(t *testing.T) {
	base := t.TempDir()
	run, err := NewRunIn(base)
	if err != nil {
		t.Fatal(err)
	}
	if run.Dir == "" {
		t.Fatal("empty run dir")
	}
	file, err := run.Create("apt-neovim")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("hello"); err != nil {
		t.Fatal(err)
	}
	file.Close()

	data, err := os.ReadFile(filepath.Join(run.Dir, "apt-neovim.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("content = %q", data)
	}
}
