package layoutdbgtest_test

import (
	"bytes"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/layoutdbg/layoutdbgtest"
)

func TestCapture_RecordsEveryDecisionInOrder(t *testing.T) {
	ctx, log := layoutdbgtest.Capture()
	require.True(t, layoutdbg.Armed(ctx))
	layoutdbg.Decision(ctx, "first", "phase", "route", "exits", 3)
	layoutdbg.Decision(ctx, "second", "phase", "place")

	recs := layoutdbgtest.Records(t, log)
	require.Len(t, recs, 2)
	assert.Equal(t, "first", recs[0]["decision"])
	assert.Equal(t, "route", recs[0]["phase"])
	assert.InDelta(t, 3, recs[0]["exits"], 0)
	assert.Equal(t, "second", recs[1]["decision"])

	type record struct {
		Decision, Phase string
		Exits           int
	}
	assert.Equal(t, []record{{"first", "route", 3}, {"second", "place", 0}}, layoutdbgtest.Decode[record](t, log))
}

func TestRecords_AnEmptyLogHasNone(t *testing.T) {
	_, log := layoutdbgtest.Capture()
	assert.Empty(t, layoutdbgtest.Records(t, log))
}

// failing is a test that records a failure instead of failing its caller.
type failing struct {
	testing.TB
	failed bool
}

func (f *failing) Helper()               {}
func (f *failing) Errorf(string, ...any) { f.failed = true }
func (f *failing) FailNow()              { f.failed = true; runtime.Goexit() }

func TestDecode_ALineThatIsNotARecordFails(t *testing.T) {
	for _, tt := range []struct {
		name, log string
		fails     bool
	}{
		{"records", `{"decision":"a"}` + "\n" + `{"decision":"b"}` + "\n", false},
		{"a line of text", `{"decision":"a"}` + "\nnot a record\n", true},
		{"a field of another type", `{"decision":1}` + "\n", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := &failing{TB: t}
			done := make(chan struct{})
			go func() {
				defer close(done)
				layoutdbgtest.Decode[struct{ Decision string }](f, bytes.NewBufferString(tt.log))
			}()
			<-done
			assert.Equal(t, tt.fails, f.failed)
		})
	}
}
