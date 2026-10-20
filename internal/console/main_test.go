package console

import (
	"os"
	"testing"
	"time"
)

// The console's tests run on one station, whatever machine they run on:
// the clock in the zone the files of record were written in.
func TestMain(m *testing.M) {
	time.Local = time.FixedZone("PDT", -7*60*60)
	os.Exit(m.Run())
}
