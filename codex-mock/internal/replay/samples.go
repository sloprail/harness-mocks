package replay

import (
	"path/filepath"
	"sort"
)

// sampleDirs are the samples of the run in dir, oldest first.
func sampleDirs(dir string) []string {
	samples, _ := filepath.Glob(filepath.Join(dir, "samples", "*"))
	sort.Strings(samples)
	return samples
}

// sampleDir is the latest sample of the run in dir, the one the model's turns
// are read from; empty when it has none.
func sampleDir(dir string) string {
	if samples := sampleDirs(dir); len(samples) > 0 {
		return samples[len(samples)-1]
	}
	return ""
}
