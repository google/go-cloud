// Copyright 2019 The Go Cloud Authors
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

// +build !windows

package main

import (
	"os"
	"os/signal"

	"golang.org/x/sys/unix"
)

// signalGracefulShutdown sends an OS signal to a process that gives the process
// the most ability to clean up before exiting. On Unix systems, this is
// SIGTERM. On Windows, it can only kill the subprocess.
func signalGracefulShutdown(p *os.Process) error {
	return p.Signal(unix.SIGTERM)
}

// interruptSignals returns the set of interrupt signals to listen for.
func interruptSignals() []os.Signal {
	return []os.Signal{unix.SIGTERM, unix.SIGINT}
}

// notifyUserSignal1 returns a channel that receives SIGUSR1 on Unix systems.
// On Windows, it returns a nil channel.
func notifyUserSignal1() (_ <-chan os.Signal, stop func()) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, unix.SIGUSR1)
	return c, func() { signal.Stop(c) }
}
