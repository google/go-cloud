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

// Helper tool for creating new releases of the Go CDK. See -help for details.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// TODO(eliben): add bumpversion flag that bumps version by 1, or to a version
// given on the command line
var helpText string = `
Helper tool for creating new releases of the Go CDK.

Automates the modifications required in the project's
go.mod files to create an test new releases.

The tool processes all modules listed in the 'allmodules'
file. For each module it handles all dependencies on
other gocloud.dev modules.

-replace adds 'replace' directives to point to local versions
for testing. -dropreplace removes these directives.

Run it from the root directory of the project.

Usage:
`

var doReplace = flag.Bool("replace", false, "add replace directives")
var doDropReplace = flag.Bool("dropreplace", false, "drop replace directives")

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

func runOnGomod(path string) {
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
				reqPath = strings.TrimPrefix(r.Path, base)
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

			if *doReplace {
				cmdCheck(fmt.Sprintf("go mod edit -replace=%s=%s %s", r.Path, rel, gomodPath))
			} else if *doDropReplace {
				cmdCheck(fmt.Sprintf("go mod edit -dropreplace=%s %s", r.Path, gomodPath))
			}
		}
	}
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), helpText)
		flag.PrintDefaults()
	}

	flag.Parse()
	if *doReplace && *doDropReplace {
		log.Fatal("Expected only one of -replace/-dropreplace")
	}

	f, err := os.Open("allmodules")
	if err != nil {
		log.Fatal(err)
	}

	input := bufio.NewScanner(f)
	input.Split(bufio.ScanLines)
	for input.Scan() {
		if len(input.Text()) > 0 && !strings.HasPrefix(input.Text(), "#") {
			runOnGomod(input.Text())
		}
	}

	if input.Err() != nil {
		log.Fatal(input.Err())
	}
}
