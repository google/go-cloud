package internal

import (
	"context"
	"time"

	"github.com/google/go-cloud/runtimevar/driver"
)

// Pinger runs a function that returns a Variable and an error after every waitTime.
// Pinger blocks until the function returns a non-nil variable or non-nil error, or
// the context is canceled.
func Pinger(ctx context.Context, f func(context.Context) (*driver.Variable, error), waitTime time.Duration) (driver.Variable, error) {
	zeroVar := driver.Variable{}
	// If the context is already canceled, just return.
	if ctx.Err() != nil {
		return zeroVar, ctx.Err()
	}

	t := time.NewTicker(waitTime)
	defer t.Stop()

	// Run ping() now as ticker doesn't perform an instant tick.
	// If there's either an error or a new value, return.
	if variable, err := f(ctx); err != nil || variable != nil {
		v := zeroVar
		if variable != nil {
			v = *variable
		}
		return v, err
	}

	for {
		select {
		case <-t.C:
			variable, err := f(ctx)
			if err != nil {
				return zeroVar, err
			}
			if variable != nil {
				return *variable, nil
			}
		case <-ctx.Done():
			return zeroVar, ctx.Err()
		}
	}
}
