// Copyright 2018 The Go Cloud Development Kit Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package sqlhealth provides a health check for a SQL database connection.
package sqlhealth // import "gocloud.dev/server/health/sqlhealth"

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Checker checks the health of a SQL database.
type Checker struct {
	cancel context.CancelFunc

	stopped <-chan struct{}
	healthy bool
}

// New starts a new asynchronous ping of the SQL database. Pings will be sent
// until one succeeds or Stop is called, whichever comes first.
func New(db *sql.DB) *Checker {
	// We create a context here because we are detaching.
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	c := &Checker{
		cancel:  cancel,
		stopped: stopped,
	}
	go func() {
		var timer *time.Timer
		defer func() {
			if timer != nil {
				timer.Stop()
			}
			close(stopped)
		}()

		wait := 250 * time.Millisecond
		const maxWait = 30 * time.Second
		for {
			if err := db.PingContext(ctx); err == nil {
				c.healthy = true
				return
			}
			if timer == nil {
				timer = time.NewTimer(wait)
			} else {
				// Timer already fired, so resetting does not race.
				timer.Reset(wait)
			}
			select {
			case <-timer.C:
				if wait < maxWait {
					// Back off next ping.
					wait *= 2
					if wait > maxWait {
						wait = maxWait
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return c
}

// CheckHealth returns nil iff the ping started by New has returned
// success.
func (c *Checker) CheckHealth() error {
	select {
	case <-c.stopped:
		if !c.healthy {
			return errors.New("ping stopped before becoming healthy")
		}
		return nil
	default:
		return errors.New("still pinging database")
	}
}

// Stop stops any ongoing ping of the database.
func (c *Checker) Stop() {
	c.cancel()
	<-c.stopped
}
