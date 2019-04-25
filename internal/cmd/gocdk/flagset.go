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

package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"golang.org/x/xerrors"
)

// flagSet wraps flag.FlagSet to suppress printing errors to the flag set's
// output and to include a custom help flag.
type flagSet struct {
	flag.FlagSet
	help bool
}

func newFlagSet(pctx *processContext, name string) *flagSet {
	set := new(flagSet)
	set.Init(name, flag.ContinueOnError)
	set.Usage = func() {}
	set.SetOutput(ioutil.Discard)
	helpVar := &helpFlag{
		name:    name,
		flagSet: set,
		output:  pctx.stderr,
	}
	set.Var(helpVar, "help", "displays this help message")
	return set
}

func (set *flagSet) Parse(args []string) error {
	err := set.FlagSet.Parse(args)
	if set.help {
		// The flag package wraps any error from Value.Set, so we can't check for
		// error identity. Preserve ErrHelp
		return flag.ErrHelp
	}
	return err
}

type helpFlag struct {
	name    string
	flagSet *flagSet
	output  io.Writer
}

func (help helpFlag) String() string {
	return ""
}

func (help helpFlag) Set(string) error {
	help.flagSet.SetOutput(help.output)
	fmt.Fprintf(help.output, "Usage of %s:\n", help.name)
	help.flagSet.PrintDefaults()
	help.flagSet.SetOutput(ioutil.Discard) // Prevent from printing error we return.

	help.flagSet.help = true
	// Exact error unimportant, but need to stop parsing.
	return flag.ErrHelp
}

func (help helpFlag) IsBoolFlag() bool {
	return true
}

// usageError indicates an invalid invocation of gocdk.
type usageError struct {
	msg    string
	frame  xerrors.Frame
	cause  error
	unwrap bool
}

// usagef formats an error message string and returns a value of type
// usageError. The error message will always have "usage: " prepended to it.
func usagef(format string, args ...interface{}) error {
	var cause error
	const wrapSuffix = ": %w"
	unwrap := strings.HasSuffix(format, wrapSuffix)
	isValue := strings.HasSuffix(format, ": %v") || strings.HasSuffix(format, ": %s")
	if len(args) > 0 && (unwrap || isValue) {
		var ok bool
		if cause, ok = args[len(args)-1].(error); ok {
			// Remove everything after and including last colon from
			// format and arguments.
			format = format[:len(format)-len(wrapSuffix)]
			args = args[:len(args)-1]
		}
	}
	return usageError{
		msg:    fmt.Sprintf(format, args...),
		frame:  xerrors.Caller(1),
		cause:  cause,
		unwrap: unwrap,
	}
}

func (ue usageError) Error() string {
	if ue.cause != nil {
		return "usage: " + ue.msg + ": " + ue.cause.Error()
	}
	return "usage: " + ue.msg
}

func (ue usageError) Unwrap() error {
	if !ue.unwrap {
		return nil
	}
	return ue.cause
}

func (ue usageError) FormatError(p xerrors.Printer) error {
	p.Printf("usage: %s", ue.msg)
	if p.Detail() {
		ue.frame.Format(p)
	}
	return ue.cause
}

func (ue usageError) Format(f fmt.State, c rune) {
	xerrors.FormatError(ue, f, c)
}
