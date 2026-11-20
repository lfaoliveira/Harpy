package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lfaoliveira/harpy/internal/config"
)

func testApp() (*application, *bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	return &application{
		stdin:  strings.NewReader(""),
		stdout: stdout,
		stderr: stderr,
	}, stdout, stderr
}

func TestInitStdout(t *testing.T) {
	app, stdout, _ := testApp()
	code, err := app.dispatch([]string{"init", "--stdout"})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if stdout.String() != config.DefaultTemplate {
		t.Fatal("--stdout does not match the embedded template")
	}
}

func TestInitWritesAndRefusesOverwrite(t *testing.T) {
	app, _, _ := testApp()
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")

	code, err := app.dispatch([]string{"init", "--path", path})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if _, _, err := config.Load(path); err != nil {
		t.Fatalf("written config does not parse: %v", err)
	}

	if code, err := app.dispatch([]string{"init", "--path", path}); err == nil || code == 0 {
		t.Fatal("expected refusal to overwrite")
	}

	if code, err := app.dispatch([]string{"init", "--path", path, "--force"}); err != nil || code != 0 {
		t.Fatalf("--force failed: code=%d err=%v", code, err)
	}
}

func TestVersionJSON(t *testing.T) {
	app, stdout, _ := testApp()
	code, err := app.dispatch([]string{"version", "--json"})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}

	var envelope struct {
		Schema  int    `json:"schema"`
		Command string `json:"command"`
		Data    struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if envelope.Schema != 1 || envelope.Command != "version" || envelope.Data.Version == "" {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
}

func TestUnknownCommand(t *testing.T) {
	app, _, _ := testApp()
	code, err := app.dispatch([]string{"nope"})
	if err == nil || code != 1 {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func TestHelp(t *testing.T) {
	app, stdout, _ := testApp()
	code, err := app.dispatch([]string{"help"})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if !strings.Contains(stdout.String(), "harpy plan") {
		t.Fatal("help does not mention plan")
	}
}

func TestManagersJSON(t *testing.T) {
	app, stdout, _ := testApp()
	code, err := app.dispatch([]string{"managers", "--json"})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}

	var envelope struct {
		Schema  int    `json:"schema"`
		Command string `json:"command"`
		Data    struct {
			OS       string `json:"os"`
			Managers []struct {
				Name string `json:"name"`
				OS   string `json:"os"`
			} `json:"managers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if envelope.Schema != 1 || envelope.Command != "managers" || envelope.Data.OS == "" {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
	if len(envelope.Data.Managers) == 0 {
		t.Fatal("no host managers reported")
	}
}

func TestInitStdoutAndJSONConflict(t *testing.T) {
	app, _, _ := testApp()
	code, err := app.dispatch([]string{"init", "--stdout", "--json"})
	if err == nil || code == 0 {
		t.Fatal("expected conflict error")
	}
}

func TestInitFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions do not apply on Windows")
	}
	app, _, _ := testApp()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if _, err := app.dispatch([]string{"init", "--path", path}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}
