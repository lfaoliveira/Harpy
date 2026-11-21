package manager

import (
	"errors"
	"runtime"
	"testing"
)

func TestWingetExitCodes(t *testing.T) {
	tests := []struct {
		code   int
		target error
	}{
		{wingetAlreadyInstalled, ErrAlreadyInstalled},
		{wingetAlreadyInstalled32, ErrAlreadyInstalled},
		{wingetNoUpdate, ErrNoUpdate},
		{3010, ErrRebootRequired},
		{1641, ErrRebootRequired},
	}
	for _, tc := range tests {
		err := wrapExit(&CommandError{Name: "winget", Code: tc.code}, wingetExitCodes)
		if !errors.Is(err, tc.target) {
			t.Errorf("code %d: got %v, want %v", tc.code, err, tc.target)
		}
	}

	err := wrapExit(&CommandError{Name: "winget", Code: 1}, wingetExitCodes)
	if errors.Is(err, ErrAlreadyInstalled) {
		t.Fatalf("generic failure classified as already installed: %v", err)
	}
}

func TestManagerAliases(t *testing.T) {
	opts := Options{}
	for alias, want := range map[string]string{
		"winget":     "winget",
		"choco":      "chocolatey",
		"chocolatey": "chocolatey",
		"apt-get":    "apt",
		"pacman":     "pacman",
	} {
		mgr, ok := ByName(alias, opts)
		if !ok {
			t.Fatalf("%q not found", alias)
		}
		if mgr.Name() != want {
			t.Fatalf("%q -> %q, want %q", alias, mgr.Name(), want)
		}
	}
	if _, ok := ByName("brew", opts); ok {
		t.Fatal("brew should not be known")
	}
}

func TestHostManagers(t *testing.T) {
	for _, mgr := range Host(Options{}) {
		if mgr.OS() != runtime.GOOS {
			t.Fatalf("%s has OS %q, host is %q", mgr.Name(), mgr.OS(), runtime.GOOS)
		}
	}
	order := HostOrder()
	if len(order) == 0 {
		t.Fatal("empty host order")
	}
}

func TestCommandErrorLastLine(t *testing.T) {
	err := &CommandError{
		Name: "apt-get",
		Code: 100,
		Stderr: `E: Could not get lock /var/lib/dpkg/lock-frontend
E: Unable to acquire the dpkg frontend lock
`,
	}
	if got := err.LastLine(); got != "E: Unable to acquire the dpkg frontend lock" {
		t.Fatalf("LastLine = %q", got)
	}
}
