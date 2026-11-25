package platform

import (
	"github.com/lfaoliveira/harpy/src/internal/domain"
	"github.com/lfaoliveira/harpy/src/internal/logx"
)

// LogStore implements domain.LogStore on the host filesystem.
type LogStore struct{}

// NewLogStore returns a filesystem-backed log store.
func NewLogStore() LogStore { return LogStore{} }

// NewRun creates a log directory under the per-user state directory.
func (LogStore) NewRun() (domain.LogRun, error) {
	base, err := LogsDir()
	if err != nil {
		return nil, err
	}
	run, err := logx.NewRunIn(base)
	if err != nil {
		return nil, err
	}
	return logRun{run: run}, nil
}

// OpenRun binds a log run to an existing directory.
func (LogStore) OpenRun(dir string) domain.LogRun {
	return logRun{run: &logx.Run{Dir: dir}}
}

type logRun struct{ run *logx.Run }

func (r logRun) Dir() string { return r.run.Dir }

func (r logRun) Create(name string) (domain.Log, error) {
	file, err := r.run.Create(name)
	if err != nil {
		return nil, err
	}
	return file, nil
}
