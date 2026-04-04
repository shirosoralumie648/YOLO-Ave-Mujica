package artifacts

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")
var ErrArtifactNotReady = errors.New("artifact not ready")

func wrapArtifactNotFound(id int64) error {
	return fmt.Errorf("artifact %d: %w", id, ErrNotFound)
}

type artifactStateError struct {
	artifactID int64
	status     string
	action     string
}

func (e artifactStateError) Error() string {
	if e.action != "" {
		return fmt.Sprintf("artifact %d is %s and cannot be %s", e.artifactID, e.status, e.action)
	}
	return fmt.Sprintf("artifact %d is %s", e.artifactID, e.status)
}

func (e artifactStateError) Unwrap() error {
	return ErrArtifactNotReady
}
