package forum

import (
	"errors"
	"fmt"
)

// Test-only helpers mirroring the wrap format used in tags.go so the
// errors.Is assertion tracks the production wrapping verbatim.
func wrapTagCapForTest(maxTags int32) error {
	return fmt.Errorf("%w: thread already has the maximum of %d tags", ErrTooManyTags, maxTags)
}

func wrapTagCapBulkForTest(maxTags int32) error {
	return fmt.Errorf("%w: adding these tags would exceed the maximum of %d tags per thread", ErrTooManyTags, maxTags)
}

func errorIsTagCapForTest(err error) bool { return errors.Is(err, ErrTooManyTags) }
