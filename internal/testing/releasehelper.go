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
	fmt.Println("Processing", path)
	modInfo := parseModuleInfo(path)

	for _, r := range modInfo.Require {
		// Find requirements on modules within the gocloud.dev tree.
		if strings.HasPrefix(r.Path, "gocloud.dev") {
			// Find the relative path from 'path' (the module file we're processing)
			// and the module required here.
			var reqPath string
			if r.Path == "gocloud.dev" {
				reqPath = "."
			} else {
				reqPath = r.Path[12:]
			}
			rel, err := filepath.Rel(filepath.Dir(path), reqPath)
			if err != nil {
				log.Fatal(err)
			}
			if strings.HasSuffix(rel, "/.") {
				rel, _ = filepath.Split(rel)
			}

			if *doReplace {
				cmdCheck(fmt.Sprintf("go mod edit -replace=%s=%s %s", r.Path, rel, path))
			} else if *doDropReplace {
				cmdCheck(fmt.Sprintf("go mod edit -dropreplace=%s %s", r.Path, path))
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
