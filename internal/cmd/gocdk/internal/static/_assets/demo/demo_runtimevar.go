// This file demonstrates basic usage of the runtimevvar.Variable portable type.
//
// It initializes a runtimevar.Variable URL based on the environment variable
// RUNTIMEVAR_VARIABLE_URL, and then registers handlers for "/demo/runtimevar"
// on http.DefaultServeMux.

package main

import (
	"context"
	"html/template"
	"net/http"
	"os"
	"time"

	"gocloud.dev/runtimevar"
	_ "gocloud.dev/runtimevar/awsparamstore"
	_ "gocloud.dev/runtimevar/blobvar"
	_ "gocloud.dev/runtimevar/constantvar"
	_ "gocloud.dev/runtimevar/filevar"
	_ "gocloud.dev/runtimevar/gcpruntimeconfig"
	_ "gocloud.dev/runtimevar/httpvar"
)

// Package variables for the runtimevar.Variable URL, and the initialized
// runtimevar.Variable.
var (
	variableURL string
	variable    *runtimevar.Variable
	variableErr error
)

func init() {
	// Register the handler. See https://golang.org/pkg/net/http/.
	http.HandleFunc("/demo/runtimevar/", runtimevarHandler)

	// Initialize the runtimevar.Variable using a URL from the environment,
	// defaulting to an in-memory constant driver.
	variableURL = os.Getenv("RUNTIMEVAR_VARIABLE_URL")
	if variableURL == "" {
		variableURL = "constant://?val=my-variable&decoder=string"
	}
	variable, variableErr = runtimevar.OpenVariable(context.Background(), variableURL)
}

// runtimevarData holds the input for the demo page. The page handler will
// initialize the struct and pass it to the template.
type runtimevarData struct {
	URL      string
	Err      error
	Snapshot *runtimevar.Snapshot
}

// runtimevarTemplate is the template for /demo/runtimevar. See runtimevarHandler.
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
    This page demonstrates the use of Go CDK's <a href="https://gocloud.dev/howto/runtimevar">runtimevar</a> package.
  </p>
  <p>
    It is currently using a runtimevar.Variable based on the URL "{{ .URL }}", which
    can be configured via the environment variable "RUNTIMEVAR_VARIABLE_URL".
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

var runtimevarTmpl = template.Must(template.New("runtimevar").Parse(runtimevarTemplate))

// runtimevarHandler is the handler for /demo/runtimevar.
func runtimevarHandler(w http.ResponseWriter, req *http.Request) {
	data := &runtimevarData{
		URL: variableURL,
	}
	defer func() {
		if err := runtimevarTmpl.Execute(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	// Verify that variable initialization succeeded.
	if variableErr != nil {
		data.Err = variableErr
		return
	}

	// Use Latest to get the current value of the variable. It will block if
	// there's never been a good value, so use a ctx with a timeout to make the
	// page render quickly.
	ctx, cancel := context.WithTimeout(req.Context(), 100*time.Millisecond)
	defer cancel()
	snapshot, err := variable.Latest(ctx)
	if err != nil {
		// We never had a good value; show an error.
		data.Err = err
		return
	}
	data.Snapshot = &snapshot
}
