package platform

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lfaoliveira/harpy/src/internal/model"
)

func TestConfigStoreWriteLoadExists(t *testing.T) {
	store := NewConfigStore()
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")

	if exists, err := store.Exists(path); err != nil || exists {
		t.Fatalf("Exists = %t, %v", exists, err)
	}
	if err := store.Write(path, []byte(model.DefaultTemplate)); err != nil {
		t.Fatal(err)
	}
	if exists, err := store.Exists(path); err != nil || !exists {
		t.Fatalf("Exists = %t, %v", exists, err)
	}

	cfg, warnings, err := store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if cfg.Version != 1 {
		t.Fatalf("version = %d", cfg.Version)
	}
}

func TestConfigStoreLoadMissing(t *testing.T) {
	_, _, err := NewConfigStore().Load(filepath.Join(t.TempDir(), "absent.yaml"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v", err)
	}
}
