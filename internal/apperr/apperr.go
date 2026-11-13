// Origin: nodelogicau/intentions-cli internal/apperr/apperr.go @ b72a395454aa
// (sentinel message renamed for episodes; Duplicate added)

// Package apperr defines the exit codes and machine-readable error codes
// shared by every front-end, and maps core errors onto them.
package apperr

import (
	"errors"
	"fmt"
)

// Exit codes (design D14 of bootstrap-episodes-cli).
const (
	ExitOK          = 0
	ExitRuntime     = 1
	ExitUsage       = 2
	ExitNotFound    = 3
	ExitCheckFailed = 4
	ExitNoWorkspace = 5
)

// Error carries an exit code and a machine-readable error code.
type Error struct {
	Code    int
	ErrCode string
	Err     error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// Usage is a caller mistake: bad flags, invalid values.
func Usage(format string, args ...any) error {
	return &Error{Code: ExitUsage, ErrCode: "usage", Err: fmt.Errorf(format, args...)}
}

// Refused is a write the format rules do not admit. It exits as a usage
// error, because the caller asked for something the format forbids, but
// carries its own code so an agent can tell the two apart.
func Refused(format string, args ...any) error {
	return &Error{Code: ExitUsage, ErrCode: "refused", Err: fmt.Errorf(format, args...)}
}

// Duplicate is a write refused because an existing episode already records
// it: a repeated (container, ordinal), or a reference already recorded from
// the same standpoint. It exits as a usage error.
func Duplicate(format string, args ...any) error {
	return &Error{Code: ExitUsage, ErrCode: "duplicate", Err: fmt.Errorf(format, args...)}
}

// Invalid is a value that does not parse or is outside its vocabulary.
func Invalid(format string, args ...any) error {
	return &Error{Code: ExitUsage, ErrCode: "invalid", Err: fmt.Errorf(format, args...)}
}

// NotFound is an id that resolves to nothing.
func NotFound(format string, args ...any) error {
	return &Error{Code: ExitNotFound, ErrCode: "not_found", Err: fmt.Errorf(format, args...)}
}

// CheckFailed is a validate or index --check verdict.
func CheckFailed(format string, args ...any) error {
	return &Error{Code: ExitCheckFailed, ErrCode: "check_failed", Err: fmt.Errorf(format, args...)}
}

// NoWorkspace is the discovery failure.
func NoWorkspace(format string, args ...any) error {
	return &Error{Code: ExitNoWorkspace, ErrCode: "no_workspace", Err: fmt.Errorf(format, args...)}
}

// Runtime is anything else.
func Runtime(err error) error {
	return &Error{Code: ExitRuntime, ErrCode: "runtime", Err: err}
}

// Sentinel errors the core raises; Classify maps them to codes.
var (
	ErrNoWorkspace = errors.New("no workspace found: run `episodes init` or set --workspace / EPISODES_WORKSPACE")
	ErrNotFound    = errors.New("not found")
)

// IsDomain reports whether err already carries an exit code.
func IsDomain(err error) bool {
	var e *Error
	return errors.As(err, &e)
}

// Classify maps any error to an *Error.
func Classify(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	switch {
	case errors.Is(err, ErrNoWorkspace):
		return &Error{Code: ExitNoWorkspace, ErrCode: "no_workspace", Err: err}
	case errors.Is(err, ErrNotFound):
		return &Error{Code: ExitNotFound, ErrCode: "not_found", Err: err}
	}
	return &Error{Code: ExitRuntime, ErrCode: "runtime", Err: err}
}
