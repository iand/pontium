package wait

import (
	prand "math/rand"
	"time"
)

// JitterDuration adds some random jitter to a duration.
// It returns a random duration between d and d * (1+j).
// If j is outside the range [0,1) it is ignored.
func JitterDuration(d time.Duration, j float64) time.Duration {
	if j < 0 || j >= 1.0 {
		return d
	}
	return d + time.Duration(float64(d)*prand.Float64()*j)
}
