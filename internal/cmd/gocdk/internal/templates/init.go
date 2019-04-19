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

// Package templates contains string literals for use by the gocdk
// command when creating user project files.
package templates

var InitTemplates = map[string]string{

	"README.md": `I'm a readme about using the cli`,

	"Dockerfile": `
# gocdk-image: 
`,

	"go.mod": `
`,

	"main.go": `
`,

	"biomes/dev/biome.json": `{,
		"serve_enabled" : true,
		"launcher" : "local"
	}
`,

	"biomes/README.md": `I'm a readme about biomes
`,

	"biomes/dev/main.tf": `
`,

	"biomes/dev/outputs.tf": `
`,

	"biomes/dev/variables.tf": `
`,

	"biomes/dev/secrets.auto.tfvars": `
`,

	".dockerignore": `
`,

	".gitignore": `*.tfvars
`,
}
