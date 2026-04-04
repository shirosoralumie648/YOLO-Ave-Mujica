package datahub

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")

func wrapNotFound(resource string, id int64) error {
	return fmt.Errorf("%s %d: %w", resource, id, ErrNotFound)
}

func wrapNamedNotFound(resource, name string) error {
	return fmt.Errorf("%s %q: %w", resource, name, ErrNotFound)
}
