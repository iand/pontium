package test

import (
	"context"
	"testing"
	"time"
)

// CtxShort returns a Context for tests that are expected to complete quickly.
// The context will be cancelled after 10 seconds or just before the test
// binary deadline (as specified by the -timeout flag when running the test), whichever
// is sooner.
func CtxShort(t *testing.T) context.Context {
	t.Helper()

	timeout := 10 * time.Second
	goal := time.Now().Add(timeout)

	deadline, ok := t.Deadline()
	if !ok {
		deadline = goal
	} else {
		deadline = deadline.Add(-time.Second)
		if deadline.After(goal) {
			deadline = goal
		}
	}

	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	t.Cleanup(cancel)
	return ctx
}
