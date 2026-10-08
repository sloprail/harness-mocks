package replay

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Every sample of a run is compared, oldest first.
func TestSampleDirsAreEverySampleOldestFirst(t *testing.T) {
	dir := t.TempDir()
	for _, s := range []string{"20261002-100000", "20261001-100000", "20261003-100000"} {
		if err := os.MkdirAll(filepath.Join(dir, "samples", s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var names []string
	for _, p := range sampleDirs(dir) {
		names = append(names, filepath.Base(p))
	}
	if want := []string{"20261001-100000", "20261002-100000", "20261003-100000"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("got %v, want %v", names, want)
	}
}
