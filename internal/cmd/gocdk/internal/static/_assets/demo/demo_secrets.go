// This file demonstrates basic usage of the secrets.Keeper portable type.
//
// It initializes a secrets.Keeper URL based on the environment variable
// SECRETS_KEEPER_URL, and then registers handlers for "/demo/secrets" on
// http.DefaultServeMux.

package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"os"

	"gocloud.dev/secrets"
	_ "gocloud.dev/secrets/awskms"
	_ "gocloud.dev/secrets/azurekeyvault"
	_ "gocloud.dev/secrets/gcpkms"
	_ "gocloud.dev/secrets/hashivault"
	_ "gocloud.dev/secrets/localsecrets"
)

// Package variables for the secrets.Keeper URL, and the initialized secrets.Keeper.
var (
	keeperURL string
	keeper    *secrets.Keeper
	keeperErr error
)

func init() {
	// Register handlers. See https://golang.org/pkg/net/http/.
	http.HandleFunc("/demo/secrets/", secretsEncryptHandler)
	http.HandleFunc("/demo/secrets/encrypt", secretsEncryptHandler)
	http.HandleFunc("/demo/secrets/decrypt", secretsDecryptHandler)

	// Initialize the secrets.Keeper using a URL from the environment, defaulting
	// to an local provider that use in-process encryption/decryption.
	keeperURL = os.Getenv("SECRETS_KEEPER_URL")
	if keeperURL == "" {
		keeperURL = "base64key://smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4="
	}
	keeper, keeperErr = secrets.OpenKeeper(context.Background(), keeperURL)
}

// secretsData holds the input for the demo pages. The page handlers will
// initialize the struct and pass it to the templates.
type secretsData struct {
	URL string
	Err error

	In     string // user input
	Out    string // output
	Base64 bool
}

const (
	secretsTemplatePrefix = `
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <title>gocloud.dev/secrets demo</title>
</head>
<body>
  <p>
    This page demonstrates the use of Go CDK's <a href="https://gocloud.dev/howto/secrets">secrets</a> package.
  </p>
  <p>
    It is currently using a secrets.Keeper based on the URL "{{ .URL }}", which
    can be configured via the environment variable "SECRETS_KEEPER_URL".
  </p>
  <ul>
    <li><a href="./encrypt">Encrypt</a> using the secrets.Keeper</li>
    <li><a href="./decrypt">Decrypt</a> using the secrets.Keeper</li>
  </ul>
  {{if .Err}}
    <p><strong>{{ .Err }}</strong></p>
  {{end}}`

	secretsTemplateSuffix = `
</body>
</html>`

	// secretsEncryptTemplate is the template for /demo/secrets and /demo/secrets/encrypt. See secretsEncryptHandler.
	// Input: *secretsData.
	secretsEncryptTemplate = secretsTemplatePrefix + `
  <form method="POST">
    <p><label>
      Enter plaintext data to encrypt:
      <br/>
      <textarea rows="4" cols="50" name="plaintext">{{ .In }}</textarea>
    </label></p>
    <label><p>
      <input type="checkbox" name="base64" value="true" {{if .Base64}}checked{{end}}>
      The data above is base64 encoded
    </label></p>
    <input type="submit" value="Encrypt!">
  </form>
  {{if .Out}}
    <p><label>
      Encrypted result (base64 encoded):
      <br/>
      <textarea rows="4" cols="50" readonly="true">{{ .Out }}</textarea>
    </label></p>
    <div>
      <form method="POST" action="./decrypt">
        <input type="hidden" name="ciphertext" value="{{ .Out }}">
        <input type="hidden" name="base64" value="{{ .Base64 }}">
        <input type="submit" value="Decrypt it">
      </form>
    </div>
  {{end}}` + secretsTemplateSuffix

	// secretsDecryptTemplate is the template for /demo/secrets/decrypt. See secretsDecryptHandler.
	// Input: *secretsData.
	secretsDecryptTemplate = secretsTemplatePrefix + `
  <form method="POST">
    <p><label>
      Enter base64-encoded data to decrypt:
      <br/>
      <textarea rows="4" cols="50" name="ciphertext">{{ .In }}</textarea>
    </label></p>
    <p><label>
      <input type="checkbox" name="base64" value="true" {{if .Base64}}checked{{end}}>
      Show result base64-encoded
    </label></p>
    <input type="submit" value="Decrypt!">
  </form>
  {{if .Out}}
    <p><label>
      Decrypted result:
      <br/>
      <textarea rows="4" cols="50" readonly="true">{{ .Out }}</textarea>
    </label></p>
    <div>
      <form method="POST" action="./encrypt">
        <input type="hidden" name="plaintext" value="{{ .Out }}">
        <input type="hidden" name="base64" value="{{ .Base64 }}">
        <input type="submit" value="Encrypt it">
      </form>
    </div>
  {{end}}` + secretsTemplateSuffix
)

var (
	secretsEncryptTmpl = template.Must(template.New("secrets encrypt").Parse(secretsEncryptTemplate))
	secretsDecryptTmpl = template.Must(template.New("secrets decrypt").Parse(secretsDecryptTemplate))
)

// secretsEncryptHandler is the handler for /demo/secrets/ and /demo/secrets/encrypt.
//
// It shows a form that allows the user to enter some data to be encrypted.
func secretsEncryptHandler(w http.ResponseWriter, req *http.Request) {
	data := &secretsData{
		URL:    keeperURL,
		In:     req.FormValue("plaintext"),
		Base64: req.FormValue("base64") == "true",
	}
	defer func() {
		if err := secretsEncryptTmpl.Execute(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	// Verify that keeper initialization succeeded.
	if keeperErr != nil {
		data.Err = keeperErr
		return
	}

	// For GET, render the form.
	if req.Method == http.MethodGet {
		return
	}

	// POST.
	// The string to be encrypted is in data.In.
	// If the user checked the "base64" box, then base64-decode it.
	var in []byte
	if data.Base64 {
		var err error
		in, err = base64.StdEncoding.DecodeString(data.In)
		if err != nil {
			data.Err = fmt.Errorf("Plaintext data was not valid Base64: %v", err)
			return
		}
	} else {
		in = []byte(data.In)
	}

	// Encrypt the input.
	encrypted, err := keeper.Encrypt(req.Context(), in)
	if err != nil {
		data.Err = fmt.Errorf("Failed to encrypt: %v", err)
		return
	}

	// Always show the result base64-encoded.
	data.Out = base64.StdEncoding.EncodeToString(encrypted)
}

// secretsDecryptHandler is the handler for /demo/secrets/decrypt.
//
// It shows a form that allows the user to enter some data to be decrypted.
func secretsDecryptHandler(w http.ResponseWriter, req *http.Request) {
	data := &secretsData{
		URL:    keeperURL,
		In:     req.FormValue("ciphertext"),
		Base64: req.FormValue("base64") == "true",
	}
	defer func() {
		if err := secretsDecryptTmpl.Execute(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	// Verify that keeper initialization succeeded.
	if keeperErr != nil {
		data.Err = keeperErr
		return
	}

	// For GET, render the form.
	if req.Method == http.MethodGet {
		return
	}

	// POST.
	// The string to be decrypted is in data.In, base64-encoded.
	in, err := base64.StdEncoding.DecodeString(data.In)
	if err != nil {
		data.Err = fmt.Errorf("Ciphertext data was not valid Base64: %v", err)
		return
	}

	// Decrypt it.
	decrypted, err := keeper.Decrypt(req.Context(), in)
	if err != nil {
		data.Err = fmt.Errorf("Failed to decrypt: %v", err)
		return
	}

	// If the user checked the "base64" box, then base64-encode the output.
	if data.Base64 {
		data.Out = base64.StdEncoding.EncodeToString(decrypted)
	} else {
		data.Out = string(decrypted)
	}
}
