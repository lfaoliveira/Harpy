package platform

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/lfaoliveira/harpy/src/internal/model"
)

// ConfigStore owns config-file access and decoding.
type ConfigStore struct{}

// NewConfigStore returns a config store bound to the host filesystem.
func NewConfigStore() ConfigStore { return ConfigStore{} }

// Path returns the default per-user config path.
func (ConfigStore) Path() (string, error) { return ConfigPath() }

// Load reads and validates a config file.
func (ConfigStore) Load(filename string) (*model.Config, []string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, nil, err
	}
	return model.Parse(data)
}

// Exists reports whether path exists.
func (ConfigStore) Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// Write creates parent directories and writes data to path.
func (ConfigStore) Write(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
