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

// Command gatherexamples extracts examples in a Go module into a JSON-formatted
// object. This is used as input for building the Go CDK Hugo website.
//
// Examples must include a comment
// // PRAGMA(gocloud.dev): This example is used on gocloud.dev; please ignore PRAGMA comments.
// somewhere in the function body in order to be included in this tool's output.
//
// gatherexamples does some minimal rewriting of the example source code for
// presentation:
//
//   - Any imports the example uses will be prepended to the code.
//   - log.Fatal(err) -> return err
//   - A comment line "// PRAGMA(gocloud.dev): Skip until next blank line." will
//     remove any code up to the next blank line. This is intended for
//     compiler-mandated setup like `ctx := context.Background()`.
//   - A comment line "// PRAGMA(gocloud.dev): Skip rest of function." will
//     remove any code until the end of the function. This is intended for
//     compiler-mandated assignments like `_ = bucket`.
//   - A comment line "// PRAGMA(gocloud.dev): Add a blank import: _ "example.com/foo""
//     will add the blank import to the example's imports.
//
// The key of each JSON object entry will be the import path of the package,
// followed by a dot ("."), followed by the name of the example function. The
// value of each JSON object entry is an object like
// {"imports": "import (\n\t\"fmt\"\n)", "code": "/* ... */"}. These are
// separated so that templating can format or show them separately.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/printer"
	"go/types"
	"os"
	"sort"
	"strconv"
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
	pattern := flag.String("pattern", "./...", "Go package pattern to use at each directory argument")
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2) // matches with flag package
	}

	// Load packages in each module named on the command line and find
	// all examples.
	allExamples := make(map[string]example)
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
		examples, err := gather(pkgs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gatherexamples: gather: %v", err)
			os.Exit(1)
		}
		for exampleName, ex := range examples {
			allExamples[exampleName] = ex
		}
	}

	// Write all examples as a JSON object.
	data, err := json.MarshalIndent(allExamples, "", "\t")
	if err != nil {
		fmt.Fprintf(os.Stderr, "gatherexamples: generate JSON: %v\n", err)
		os.Exit(1)
	}
	data = append(data, '\n')
	if _, err := os.Stdout.Write(data); err != nil {
		fmt.Fprintf(os.Stderr, "gatherexamples: write output: %v\n", err)
		os.Exit(1)
	}
}

const gatherLoadMode packages.LoadMode = packages.NeedName |
	packages.NeedFiles |
	packages.NeedTypes |
	packages.NeedSyntax |
	packages.NeedTypesInfo |
	packages.NeedImports |
	// TODO(light): We really only need name from deps, but there's no way to
	// specify that in the current go/packages API. This sadly makes this program
	// 10x slower. Reported as https://github.com/golang/go/issues/31699.
	packages.NeedDeps

// pragmaPrefix is the prefix for all comments in examples that are used as
// directives for formatting.
const pragmaPrefix = "// PRAGMA(gocloud.dev): "

// inclusionComment is the comment used to signify whether the example should be
// included in the output.
const inclusionComment = pragmaPrefix + "This example is used on gocloud.dev; please ignore PRAGMA comments."

type example struct {
	Imports string `json:"imports"`
	Code    string `json:"code"`
}

// gather extracts the code from the example functions in the given packages
// and returns a map like the one described in the package documentation.
func gather(pkgs []*packages.Package) (map[string]example, error) {
	examples := make(map[string]example)
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				// Determine whether this declaration is an example function.
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || !strings.HasPrefix(fn.Name.Name, "Example") || len(fn.Type.Params.List) > 0 || len(fn.Type.Params.List) > 0 {
					continue
				}

				// Format example into string.
				sb := new(strings.Builder)
				err := format.Node(sb, pkg.Fset, &printer.CommentedNode{
					Node:     fn.Body,
					Comments: file.Comments,
				})
				if err != nil {
					return nil, err // will only occur for bad invocations of Fprint
				}
				original := sb.String()
				if !strings.Contains(original, inclusionComment) {
					// Does not contain the inclusion comment. Skip it, but first verify
					// that it doesn't contain any PRAGMA comments; only examples with
					// the inclusion comment should include pragmas.
					if strings.Contains(original, pragmaPrefix) {
						return nil, fmt.Errorf("%s in package %s has PRAGMA(s) for gatherexamples, but is not marked for inclusion with %q", fn.Name.Name, pkg.PkgPath, inclusionComment)
					}
					continue
				}
				exampleCode, blankImports := rewriteBlock(original)

				// Gather map of imported packages to overridden identifier.
				usedPackages := make(map[string]string)
				for _, path := range blankImports {
					usedPackages[path] = "_"
				}
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
				// Remove "log" import since it's almost always used for log.Fatal(err).
				delete(usedPackages, "log")

				pkgPath := strings.TrimSuffix(pkg.PkgPath, "_test")
				exampleName := pkgPath + "." + fn.Name.Name
				examples[exampleName] = example{
					Imports: formatImports(usedPackages),
					Code:    exampleCode,
				}
			}
		}
	}
	return examples, nil
}

// rewriteBlock reformats a Go block statement for display as an example.
// It also extracts any blank imports found
func rewriteBlock(block string) (_ string, blankImports []string) {
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

		// Write the line to sb, performing textual substitutions as needed.
		start := strings.IndexFunc(line, func(r rune) bool { return r != ' ' && r != '\t' })
		if start == -1 {
			// Blank.
			sb.WriteString(line)
			sb.WriteByte('\n')
			continue
		}
		const importBlankPrefix = pragmaPrefix + "Add a blank import: _ "
		indent, lineContent := line[:start], line[start:]
		switch {
		case lineContent == pragmaPrefix+"Skip until next blank line.":
			// Skip lines until we hit a blank line.
			for len(block) > 0 {
				var next string
				next, block = nextLine(block)
				if strings.TrimSpace(next) == "" {
					break
				}
			}
		case lineContent == pragmaPrefix+"Skip rest of function.":
			// Ignore remaining lines.
			break rewrite
		case lineContent == "log.Fatal(err)":
			sb.WriteString(indent)
			sb.WriteString("return err")
			sb.WriteByte('\n')
		case strings.HasPrefix(lineContent, importBlankPrefix):
			// Blank import.
			path, err := strconv.Unquote(lineContent[len(importBlankPrefix):])
			if err == nil {
				blankImports = append(blankImports, path)
			}
		case strings.Contains(lineContent, inclusionComment):
			// inclusion comment. Skip it.
		default:
			// Ordinary line, write as-is.
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
	}
	return strings.TrimSpace(sb.String()), blankImports
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
