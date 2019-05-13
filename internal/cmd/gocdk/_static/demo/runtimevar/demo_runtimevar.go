package main

import (
	"context"
	"html/template"
	"net/http"
	"os"

	"gocloud.dev/runtimevar"
	_ "gocloud.dev/runtimevar/constantvar"
	_ "gocloud.dev/runtimevar/filevar"
)

// TODO(rvangent): This file is user-visible, add many comments explaining
// how it works.

func init() {
	http.HandleFunc("/demo/runtimevar/", runtimevarHandler)
}

var variableURL string
var variable *runtimevar.Variable
var variableErr error

func init() {
	variableURL = os.Getenv("RUNTIMEVAR_VARIABLE_URL")
	if variableURL == "" {
		variableURL = "constant://?val=my-variable&decoder=string"
	}
	variable, variableErr = runtimevar.OpenVariable(context.Background(), variableURL)
}

type runtimevarData struct {
	URL      string
	Err      error
	Snapshot *runtimevar.Snapshot
}

// Input: *runtimevarData.
const runtimevarTemplate = `
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <title>gocloud.dev/runtimevar demo</title>
</head>
<body>
  <p>
    This page demonstrates the use of Go CDK's <a href="https://godoc.org/gocloud.dev/runtimevar">runtimevar</a> package.
  </p>
  <p>
    It is currently using a runtimevar.Variable based on the URL "{{ .URL }}", which
    can be configured via the environment variable "RUNTIMEVAR_VARIABLE_URL".
  </p>
  <p>
    See <a href="https://gocloud.dev/concepts/urls/">here</a> for more
    information about URLs in Go CDK APIs.
  </p>
  {{if .Err}}
    <p><strong>{{ .Err }}</strong></p>
  {{end}}
  {{if .Snapshot}}
    <p><label>
      The current value of the variable is:
      <br/>
      <textarea rows="5" cols="40" readonly="true">{{ .Snapshot.Value }}</textarea>
    </label></p>
    <p><label>
      It was last modified at: {{ .Snapshot.UpdateTime }}.
    </label></p>
  {{end}}
</body>
</html>`

var runtimevarTmpl = template.Must(template.New("runtimevar.Variable").Parse(runtimevarTemplate))

func runtimevarHandler(w http.ResponseWriter, req *http.Request) {
	input := &runtimevarData{
		URL: variableURL,
	}
	defer func() {
		if err := runtimevarTmpl.Execute(w, input); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	if variableErr != nil {
		input.Err = variableErr
		return
	}
	snapshot, err := variable.Latest(req.Context())
	if err != nil {
		input.Err = err
		return
	}
	input.Snapshot = &snapshot
}
