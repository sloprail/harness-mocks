package runner

import (
	"fmt"
	"math/rand"
	"time"
)

// probeRunID names a run with a random id seeded from the clock (rule probe).
func probeRunID() string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return fmt.Sprintf("run-%d", r.Int())
}
