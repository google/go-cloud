package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

var doReplace = flag.Bool("replace", false, "add replace directives to root")
var doDropReplace = flag.Bool("dropreplace", false, "drop replace directives to root")

func cmdCheck(s string) []byte {
	fmt.Printf(" -> %s\n", s)
	fields := strings.Fields(s)
	if len(fields) < 1 {
		log.Fatal("Expected \"command <arguments>\"")
	}
	b, err := exec.Command(fields[0], fields[1:]...).Output()
	if exiterr, ok := err.(*exec.ExitError); ok {
		log.Fatalf("%s; stderr: %s\n", err, string(exiterr.Stderr))
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
	modInfo := parseModuleInfo(path)

	for _, r := range modInfo.Require {
		if r.Path == "gocloud.dev" {
			// Find relative path needed for the replace directive.
			// TODO(eliben): handle paths to submodules here too, not only root
			fmt.Println("Found reference to gocloud.dev")
			rel, err := filepath.Rel(filepath.Dir(path), ".")
			if err != nil {
				log.Fatal(err)
			}
			dir, _ := filepath.Split(rel)

			if *doReplace {
				cmdCheck(fmt.Sprintf("go mod edit -replace=gocloud.dev=%s %s", dir, path))
			}
		}
	}
}

func main() {
	flag.Parse()
	// TODO: make sure flags make sense with each other
	f, err := os.Open("allmodules")
	if err != nil {
		log.Fatal(err)
	}

	input := bufio.NewScanner(f)
	input.Split(bufio.ScanLines)
	for input.Scan() {
		if len(input.Text()) > 0 && !strings.HasPrefix(input.Text(), "#") {
			runOnGomod(path.Join(input.Text(), "go.mod"))
		}
	}
}
