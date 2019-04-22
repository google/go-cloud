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

// Command gatherexamples extracts examples in a Go module into a JSON-formatted
// object. This is used as input for building the Go CDK Hugo website.
// Examples must include a comment that starts with "// This example is used in"
// somewhere in the function body in order to be included in this tool's output.
//
// gatherexamples does some minimal rewriting of the example source code for
// presentation:
//
//   - Any imports the example uses will be prepended to the code.
//   - log.Fatal(err) -> return err
//   - A comment line "// Variables set up elsewhere:" will remove any code up
//     to the next blank line. This is intended for compiler-mandated setup
//     like `ctx := context.Background()`.
//   - A comment line "// Ignore unused variables in example:" will remove any
//     code until the end of the function. This is intended for
//     compiler-mandated assignments like `_ = bucket`.
//
// The key of each JSON object entry will be the import path of the package,
// followed by a dot ("."), followed by the name of the example function.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/printer"
	"go/types"
	"os"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

func main() {
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintln(out, "usage: gatherexamples [options] DIR [...]")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Options:")
		flag.PrintDefaults()
	}
	outputPath := flag.String("o", "", "path to output file (default to stdout)")
	pattern := flag.String("pattern", "./...", "Go package pattern to use at each directory argument")
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2) // matches with flag package
	}

	// Load packages in each module named on the command line and find
	// all examples.
	allExamples := make(map[string]string)
	for _, dir := range flag.Args() {
		cfg := &packages.Config{
			Mode:  gatherLoadMode,
			Dir:   dir,
			Tests: true,
		}
		pkgs, err := packages.Load(cfg, *pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gatherexamples: load %s: %v\n", dir, err)
			os.Exit(1)
		}
		for exampleName, source := range gather(pkgs) {
			allExamples[exampleName] = source
		}
	}

	// Write all examples as a JSON object.
	data, err := json.MarshalIndent(allExamples, "", "\t")
	if err != nil {
		fmt.Fprintf(os.Stderr, "gatherexamples: generate JSON: %v\n", err)
		os.Exit(1)
	}
	data = append(data, '\n')

	output := os.Stdout
	if *outputPath != "" && *outputPath != "-" {
		var err error
		output, err = os.Create(*outputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gatherexamples: %v\n", err)
			os.Exit(1)
		}
		defer func() {
			if err := output.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "gatherexamples: write output: %v\n", err)
				os.Exit(1)
			}
		}()
	}
	if _, err := output.Write(data); err != nil {
		fmt.Fprintf(os.Stderr, "gatherexamples: write output: %v\n", err)
		os.Exit(1)
	}
}

const gatherLoadMode packages.LoadMode = packages.NeedName |
	packages.NeedFiles |
	packages.NeedTypes |
	packages.NeedSyntax |
	packages.NeedTypesInfo

// signifierCommentPrefix is the start of the comment used to signify whether
// the example should be included in the output.
const signifierCommentPrefix = "// This example is used in"

// gather extracts the code from the example functions in the given packages
// and returns a map like the one described in the package documentation.
func gather(pkgs []*packages.Package) map[string]string {
	examples := make(map[string]string)
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				// Determine whether this declaration is an example function.
				fn, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				if !ok || !strings.HasPrefix(fn.Name.Name, "Example") || len(fn.Type.Params.List) > 0 || len(fn.Type.Params.List) > 0 {
					continue
				}

				// Gather sorted list of imported packages.
				usedPackages := make(map[string]string)
				ast.Inspect(fn.Body, func(node ast.Node) bool {
					id, ok := node.(*ast.Ident)
					if !ok {
						return true
					}
					refPkg, ok := pkg.TypesInfo.ObjectOf(id).(*types.PkgName)
					if !ok {
						return true
					}
					overrideName := ""
					if id.Name != refPkg.Imported().Name() {
						overrideName = id.Name
					}
					usedPackages[refPkg.Imported().Path()] = overrideName
					return true
				})

				// Format example into string.
				sb := new(strings.Builder)
				err := printer.Fprint(sb, pkg.Fset, &printer.CommentedNode{
					Node:     fn.Body,
					Comments: file.Comments,
				})
				if err != nil {
					panic(err) // will only occur for bad invocations of Fprint
				}
				original := sb.String()
				if !strings.Contains(original, "\n\t"+signifierCommentPrefix) {
					// Does not contain the signifier comment. Skip it.
					continue
				}
				exampleCode := rewriteBlock(original)
				if len(usedPackages) > 0 {
					exampleCode = formatImports(usedPackages) + "\n\n" + exampleCode
				}

				pkgPath := strings.TrimSuffix(pkg.PkgPath, "_test")
				exampleName := pkgPath + "." + fn.Name.Name
				examples[exampleName] = exampleCode
			}
		}
	}
	return examples
}

// rewriteBlock reformats a Go block statement for display as an example.
func rewriteBlock(block string) string {
	// Trim block.
	block = strings.TrimPrefix(block, "{")
	block = strings.TrimSuffix(block, "}")

	// Rewrite line-by-line.
	sb := new(strings.Builder)
rewrite:
	for len(block) > 0 {
		var line string
		line, block = nextLine(block)

		// Dedent line.
		// TODO(light): In the case of a multi-line raw string literal,
		// this can produce incorrect rewrites.
		line = strings.TrimPrefix(line, "\t")

		// Check if this is line that needs rewriting.
		start := strings.IndexFunc(line, func(r rune) bool { return r != ' ' && r != '\t' })
		if start == -1 {
			// Blank.
			sb.WriteString(line)
			sb.WriteByte('\n')
			continue
		}
		indent := line[:start]
		switch line[start:] {
		case "// Variables set up elsewhere:":
			// Skip lines until we hit a blank line.
			for len(block) > 0 {
				var next string
				next, block = nextLine(block)
				if strings.TrimSpace(next) == "" {
					break
				}
			}
		case "// Ignore unused variables in example:":
			// Ignore remaining lines.
			break rewrite
		case "log.Fatal(err)":
			sb.WriteString(indent)
			sb.WriteString("return err")
			sb.WriteByte('\n')
		default:
			if !strings.HasPrefix(line[start:], signifierCommentPrefix) {
				// Ordinary line.
				sb.WriteString(line)
				sb.WriteByte('\n')
			}
		}
	}
	return strings.TrimSpace(sb.String())
}

// nextLine splits the string at the next linefeed.
func nextLine(s string) (line, tail string) {
	i := strings.IndexByte(s, '\n')
	if i == -1 {
		return s, ""
	}
	return s[:i], s[i+1:]
}

// formatImports formats a map of imports to their package identifiers into a
// Go import declaration.
func formatImports(usedPackages map[string]string) string {
	if len(usedPackages) == 0 {
		return ""
	}
	if len(usedPackages) == 1 {
		// Special case: one-line import.
		for path, id := range usedPackages {
			if id != "" {
				return fmt.Sprintf("import %s %q", id, path)
			}
			return fmt.Sprintf("import %q", path)
		}
	}
	// Typical case: multiple imports in factored declaration form.
	// Group into standard library imports then third-party imports.
	sortedStdlib := make([]string, 0, len(usedPackages))
	sortedThirdParty := make([]string, 0, len(usedPackages))
	for path := range usedPackages {
		if strings.ContainsRune(path, '.') {
			// Third-party imports almost always contain a dot for a domain name,
			// especially in GOPATH/Go modules workspaces.
			sortedThirdParty = append(sortedThirdParty, path)
		} else {
			sortedStdlib = append(sortedStdlib, path)
		}
	}
	sort.Strings(sortedStdlib)
	sort.Strings(sortedThirdParty)
	sb := new(strings.Builder)
	sb.WriteString("import (\n")
	printImports := func(paths []string) {
		for _, path := range paths {
			id := usedPackages[path]
			if id == "" {
				fmt.Fprintf(sb, "\t%q\n", path)
			} else {
				fmt.Fprintf(sb, "\t%s %q\n", id, path)
			}
		}
	}
	printImports(sortedStdlib)
	if len(sortedStdlib) > 0 && len(sortedThirdParty) > 0 {
		// Insert blank line to separate.
		sb.WriteByte('\n')
	}
	printImports(sortedThirdParty)
	sb.WriteString(")")
	return sb.String()
}
