package annotations

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrConflict   = errors.New("conflict")
	ErrValidation = errors.New("validation")
)

type typedError struct {
	kind error
	msg  string
}

func (e typedError) Error() string {
	return e.msg
}

func (e typedError) Unwrap() error {
	return e.kind
}

func newTypedError(kind error, format string, args ...any) error {
	return typedError{kind: kind, msg: fmt.Sprintf(format, args...)}
}

func newNotFoundError(format string, args ...any) error {
	return newTypedError(ErrNotFound, format, args...)
}

func newConflictError(format string, args ...any) error {
	return newTypedError(ErrConflict, format, args...)
}

func newValidationError(format string, args ...any) error {
	return newTypedError(ErrValidation, format, args...)
}
