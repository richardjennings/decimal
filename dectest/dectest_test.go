package dectest

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSuite runs every .decTest file in DECTEST_DIR (default: ./testdata).
// It ships with a small hand-written sample; run `make testdata` to fetch the
// full IBM suite, then point DECTEST_DIR at it. Cases for operations or features
// not yet implemented are skipped, so the counts are the conformance backlog.
func TestSuite(t *testing.T) {
	dir := os.Getenv("DECTEST_DIR")
	if dir == "" {
		dir = "testdata"
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.decTest"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Skipf("no .decTest files in %s (run `make testdata`)", dir)
	}

	var total Result
	for _, f := range files {
		r, err := RunFile(f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		total.Pass += r.Pass
		total.Fail += r.Fail
		total.Skip += r.Skip
		for _, msg := range r.Failures {
			if len(total.Failures) < 5000 {
				total.Failures = append(total.Failures, msg)
			}
		}
	}
	t.Logf("%d passed, %d failed, %d skipped across %d file(s)",
		total.Pass, total.Fail, total.Skip, len(files))
	for _, msg := range total.Failures {
		t.Errorf("FAIL: %s", msg)
	}
}
