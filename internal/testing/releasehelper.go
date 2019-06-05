// Copyright 2019 The Go Cloud Development Kit Authors
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

// Helper tool for creating new releases of the Go CDK. Run without arguments
// or with 'help' for details.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// TODO(eliben): add bumpversion command that bumps version by 1, or to a version
// given on the command line
var helpText string = `
Helper tool for creating new releases of the Go CDK.

Automates the modifications required in the project's
go.mod files to create an test new releases.

The tool processes all modules listed in the 'allmodules'
file. For each module it handles all dependencies on
other gocloud.dev modules.

Run it from the root directory of the repository, as follows:

$ %s <command>

Where command is one of the following:

  addreplace    adds 'replace' directives to point to local versions
                for testing.

  dropreplace   removes these directives.

  help          prints this usage message
`

func printHelp() {
	_, binName := filepath.Split(os.Args[0])
	fmt.Fprintf(os.Stderr, helpText, binName)
	fmt.Fprintln(os.Stderr)
}

// cmdCheck invokes the command given in s, echoing the invocation to stdout.
// It checks that the command was successful and returns its standard output.
// If the command returned a non-0 status, log.Fatal is invoked.
func cmdCheck(s string) []byte {
	fmt.Printf(" -> %s\n", s)
	fields := strings.Fields(s)
	if len(fields) < 1 {
		log.Fatal(`Expected "command <arguments>"`)
	}
	b, err := exec.Command(fields[0], fields[1:]...).Output()
	if exiterr, ok := err.(*exec.ExitError); ok {
		log.Fatalf("%s; stderr: %s\n", err, string(exiterr.Stderr))
	} else if err != nil {
		log.Fatal("exec.Command", err)
	}
	return b
}

// These types are taken from "go mod help edit", in order to parse the JSON
// output of `go mod edit -json`.
type GoMod struct {
	Module  Module
	Go      string
	Require []Require
	Exclude []Module
	Replace []Replace
}

type Module struct {
	Path    string
	Version string
}

type Require struct {
	Path     string
	Version  string
	Indirect bool
}

type Replace struct {
	Old Module
	New Module
}

// parseModuleInfo parses module information from a go.mod file at path.
func parseModuleInfo(path string) GoMod {
	rawJson := cmdCheck("go mod edit -json " + path)
	var modInfo GoMod
	err := json.Unmarshal(rawJson, &modInfo)
	if err != nil {
		log.Fatal(err)
	}
	return modInfo
}

// reqHandlerFunc is a callback function type invoked by runOnGomod when it
// finds 'require' lines in go.mod files that refer to our modules. It's called
// with these arguments:
//
//   gomodPath - path to the go.mod file where this 'require' was found
//   mod - name of the module being 'require'd
//   modPath - mod's location in the filesystem relative to
//             the go.mod 'require'ing it
type reqHandlerFunc func(gomodPath, mod, modPath string)

func runOnGomod(path string, reqHandler reqHandlerFunc) {
	gomodPath := filepath.Join(path, "go.mod")
	fmt.Println("Processing", gomodPath)
	modInfo := parseModuleInfo(gomodPath)

	base := "gocloud.dev"

	for _, r := range modInfo.Require {
		// Find requirements on modules within the gocloud.dev tree.
		if strings.HasPrefix(r.Path, base) {
			// Find the relative path from 'path' and the module required here.
			var reqPath string
			if r.Path == base {
				reqPath = "."
			} else {
				reqPath = strings.TrimPrefix(r.Path, base+"/")
			}
			rel, err := filepath.Rel(path, reqPath)
			if err != nil {
				log.Fatal(err)
			}
			// When path is '.', filepath.Rel will append a /. to the result and we
			// may get paths like ../../.
			if strings.HasSuffix(rel, "/.") {
				rel, _ = filepath.Split(rel)
			}

			reqHandler(gomodPath, r.Path, rel)
		}
	}
}

func gomodAddReplace(path string) {
	runOnGomod(path, func(gomodPath, mod, modPath string) {
		cmdCheck(fmt.Sprintf("go mod edit -replace=%s=%s %s", mod, modPath, gomodPath))
	})
}

func gomodDropReplace(path string) {
	runOnGomod(path, func(gomodPath, mod, modPath string) {
		cmdCheck(fmt.Sprintf("go mod edit -dropreplace=%s %s", mod, gomodPath))
	})
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(0)
	}

	var gomodHandler func(path string)
	switch os.Args[1] {
	case "help":
		printHelp()
		os.Exit(0)
	case "addreplace":
		gomodHandler = gomodAddReplace
	case "dropreplace":
		gomodHandler = gomodDropReplace
	default:
		printHelp()
		os.Exit(1)
	}

	f, err := os.Open("allmodules")
	if err != nil {
		log.Fatal(err)
	}

	input := bufio.NewScanner(f)
	input.Split(bufio.ScanLines)
	for input.Scan() {
		if len(input.Text()) > 0 && !strings.HasPrefix(input.Text(), "#") {
			gomodHandler(input.Text())
		}
	}

	if input.Err() != nil {
		log.Fatal(input.Err())
	}
}
