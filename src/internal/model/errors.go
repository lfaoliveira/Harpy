package model

import "errors"

// Sentinel errors returned by manager adapters. The domain layer maps them to
// friendly plan statuses.
var (
	ErrNoDownload       = errors.New("package manager does not support pre-download")
	ErrAlreadyInstalled = errors.New("package is already installed")
	ErrNoUpdate         = errors.New("no update available")
	ErrRebootRequired   = errors.New("reboot required")
)
