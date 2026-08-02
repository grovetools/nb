package sync

import (
	"io"

	"github.com/sirupsen/logrus"
)

// testLogger returns a discarding logger entry, so a Syncer can be exercised
// in isolation without a full service.
func testLogger() *logrus.Entry {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return l.WithField("sub-component", "syncer-test")
}
