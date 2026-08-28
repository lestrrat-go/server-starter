package supervisor

import (
	"errors"
	"sync/atomic"
)

// ErrServerClosed is recorded on a Controller when its run exits cleanly
// because the context passed to Run was cancelled. Callers should treat it
// as a successful shutdown: if err := ctrl.Wait(); err != nil &&
// !errors.Is(err, ErrServerClosed) { ... }.
var ErrServerClosed = errors.New("supervisor: server closed")

// Controller is the handle returned by Starter.Run. It is the only way to
// observe or influence a running supervisor: cancel the context passed to
// Run to stop it, and call Hangup to request a graceful worker restart.
type Controller struct {
	done   chan struct{}
	err    atomic.Pointer[error]
	hangup chan struct{}
}

// newController returns a Controller ready to be handed to a freshly
// spawned loop goroutine.
func newController() *Controller {
	return &Controller{
		done:   make(chan struct{}),
		hangup: make(chan struct{}, 1),
	}
}

// Done returns a channel that is closed when the run's loop goroutine has
// fully exited (listeners closed, pid file released).
func (c *Controller) Done() <-chan struct{} { return c.done }

// Err returns the terminal error for the run: nil before the run exits,
// ErrServerClosed after a clean context-driven shutdown, and the
// underlying failure for a genuine error.
func (c *Controller) Err() error {
	if p := c.err.Load(); p != nil {
		return *p
	}
	return nil
}

// Wait blocks until the run has exited and returns its terminal error.
// Equivalent to <-Done() then Err().
func (c *Controller) Wait() error {
	<-c.done
	return c.Err()
}

// Hangup requests a graceful worker restart, equivalent to the historical
// SIGHUP behaviour: a new worker is spawned and the old one is signalled
// once the new one is up. Hangup never blocks. Requests are coalesced while
// another request is pending or a restart is in progress. Requests received
// while old workers are draining are coalesced into one later restart.
func (c *Controller) Hangup() {
	select {
	case c.hangup <- struct{}{}:
	default:
	}
}

// setErr records err as the run's terminal error. No-op when err is nil.
// The loop goroutine calls this at most once, shortly before closing done.
func (c *Controller) setErr(err error) {
	if err != nil {
		c.err.Store(&err)
	}
}
