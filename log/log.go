// Copyright 2018 Google LLC
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

package log

import (
	"context"
	"flag"
	"fmt"
	"os"

	cloud "github.com/google/go-cloud"
)

// TODO(vsekhar): make this thread safe.

type logProviderFunc func(context.Context, string) (Logger, error)

var logProviders = make(map[string]logProviderFunc)

func defaultProvider() Logger {
	l, err := localProvider(context.Background(), "local://:stderr")
	if err != nil {
		panic(err)
	}
	return l
}

var activeLoggers = []Logger{defaultProvider()}

type logFlags bool

func (f *logFlags) String() string { return "logFlags" }

func (f *logFlags) Set(v string) error {
	if !*f {
		// The first time we set something, clear the default provider.
		activeLoggers = nil
		*f = true
	}
	return activateLogProvider(v)
}

func init() {
	flag.Var(new(logFlags), "log", "adds a destination to log to (default: local://stderr)")
}

func activateLogProvider(uri string) error {
	service, _, err := cloud.Service(uri)
	if err != nil {
		return fmt.Errorf("bad log URI: %s", uri)
	}
	p, ok := logProviders[service]
	if !ok {
		return fmt.Errorf("no log provider for: %s", uri)
	}
	provider, err := p(context.Background(), uri)
	if err != nil {
		return err
	}
	activeLoggers = append(activeLoggers, provider)
	return nil
}

func RegisterLogProvider(name string, f logProviderFunc) {
	logProviders[name] = f
}

// send sends an unstructured message to each active logger.
func send(ctx context.Context, msg string) {
	for _, l := range activeLoggers {
		l.Log(ctx, msg)
	}
}

// sync sends an unstructured message to each active logger and returns the
// first error.
func sync(ctx context.Context, msg string) error {
	for _, l := range activeLoggers {
		if err := l.LogSync(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

func Print(ctx context.Context, msg interface{}) {
	send(ctx, fmt.Sprint(msg))
}

func Printf(ctx context.Context, msg string, v ...interface{}) {
	send(ctx, fmt.Sprintf(msg, v...))
}

func Fatal(ctx context.Context, msg interface{}) {
	sync(ctx, fmt.Sprint(msg))
	os.Exit(1)
}

func Fatalf(ctx context.Context, msg string, v ...interface{}) {
	sync(ctx, fmt.Sprintf(msg, v...))
	os.Exit(1)
}
