package run

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sync/errgroup"
)

type Runnable interface {
	// Run starts running the component and blocks until the context is canceled, Shutdown is // called or a fatal error is encountered.
	Run(context.Context) error
}

type RunnableFunc func(context.Context) error

func (r RunnableFunc) Run(ctx context.Context) error { return r(ctx) }

type Group struct {
	runnables []Runnable
}

func (rg *Group) Add(r Runnable) {
	rg.runnables = append(rg.runnables, r)
}

func (rg *Group) RunAndWait(ctx context.Context) error {
	if len(rg.runnables) == 0 {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	g, ctx := errgroup.WithContext(ctx)

	for i := range rg.runnables {
		r := rg.runnables[i]
		g.Go(func() error { return r.Run(ctx) })
	}

	// Ensure components stop if we receive a terminating operating system signal.
	g.Go(func() error {
		interrupt := make(chan os.Signal, 1)
		signal.Notify(interrupt, syscall.SIGTERM, syscall.SIGINT)
		select {
		case <-interrupt:
			cancel()
		case <-ctx.Done():
		}
		return nil
	})

	// Wait for all servers to run to completion.
	if err := g.Wait(); err != nil {
		if !errors.Is(err, context.Canceled) {
			return err
		}
	}
	return nil
}
