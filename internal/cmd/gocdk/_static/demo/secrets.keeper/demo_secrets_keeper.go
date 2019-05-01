package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"os"

	"gocloud.dev/secrets"
	_ "gocloud.dev/secrets/localsecrets"
)

// TODO(rvangent): This file is user-visible, add many comments explaining
// how it works.

func init() {
	http.HandleFunc("/demo/secrets.keeper/", secretsKeeperEncryptHandler)
	http.HandleFunc("/demo/secrets.keeper/encrypt", secretsKeeperEncryptHandler)
	http.HandleFunc("/demo/secrets.keeper/decrypt", secretsKeeperDecryptHandler)
}

var keeperURL string
var keeper *secrets.Keeper
var keeperErr error

func init() {
	keeperURL = os.Getenv("SECRETS_KEEPER_URL")
	if keeperURL == "" {
		// TODO(rvangent): Remove default later.
		keeperURL = "base64key://smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4="
	}
	keeper, keeperErr = secrets.OpenKeeper(context.Background(), keeperURL)
}

type secretsKeeperData struct {
	URL string
	Err error

	In     string // user input
	Out    string // output
	Base64 bool
}

const (
	secretsKeeperTemplatePrefix = `
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <title>secrets.Keeper demo</title>
</head>
<body>
  <p>
    This page demonstrates the use of a Go CDK secrets.Keeper.
  </p>
  <p>
    It is currently using a secrets.Keeper based on the URL "{{ .URL }}", which
    can be configured via the environment variable "SECRETS_KEEPER_URL".
  </p>
  <p>
    See <a href="https://gocloud.dev/concepts/urls/">here</a> for more
    information about URLs in Go CDK APIs.
  </p>
  <ul>
    <li><a href="./encrypt">Encrypt</a> using the Keeper</li>
    <li><a href="./decrypt">Decrypt</a> using the Keeper</li>
  </ul>
  {{if .Err}}
    <p><strong>{{ .Err }}</strong></p>
  {{end}}
`
	secretsKeeperTemplateSuffix = `
</body>
</html>
`

	// Input: *secretsKeeperData.
	secretsKeeperEncryptTemplate = secretsKeeperTemplatePrefix + `
  <form>
    Enter plaintext data to encrypt:
    <br/>
    <textarea rows="4" cols="50" name="plaintext">{{ .In }}</textarea>
    <br/>
    <input type="checkbox" name="base64" value="true" {{if .Base64}}checked{{end}}>
    The data above is base64 encoded
    <br/>
    <input type="submit" value="Encrypt!">
  </form>
  {{if .Out}}
    <div>Encrypted result (base64 encoded):</div>
    <textarea rows="4" cols="50" readonly="true">{{ .Out }}</textarea>
    <div>
      <a href="./decrypt?ciphertext={{ .Out }}&base64={{ .Base64 }}">Decrypt it</a>
    </div>
  {{end}}
` + secretsKeeperTemplateSuffix

	// Input: *secretsKeeperData.
	secretsKeeperDecryptTemplate = secretsKeeperTemplatePrefix + `
  <form>
    Enter base64-encoded data to decrypt:
    <br/>
    <textarea rows="4" cols="50" name="ciphertext">{{ .In }}</textarea>
    <br/>
    <input type="checkbox" name="base64" value="true" {{if .Base64}}checked{{end}}>
    Show result base64-encoded
    <br/>
    <input type="submit" value="Decrypt!">
  </form>
  {{if .Out}}
    <div>Decrypted result:</div>
    <textarea rows="4" cols="50" readonly="true">{{ .Out }}</textarea>
    <div>
      <a href="./encrypt?plaintext={{ .Out }}&base64={{.Base64}}">Encrypt it</a>
    </div>
  {{end}}
` + secretsKeeperTemplateSuffix
)

var (
	secretsKeeperEncryptTmpl = template.Must(template.New("secrets.Keeper encrypt").Parse(secretsKeeperEncryptTemplate))
	secretsKeeperDecryptTmpl = template.Must(template.New("secrets.Keeper decrypt").Parse(secretsKeeperDecryptTemplate))
)

// secretsKeeperEncryptHandler allows the user to enter some data to be encrypted.
func secretsKeeperEncryptHandler(w http.ResponseWriter, req *http.Request) {
	input := &secretsKeeperData{
		URL:    keeperURL,
		In:     req.FormValue("plaintext"),
		Base64: req.FormValue("base64") == "true",
	}
	defer func() {
		if err := secretsKeeperEncryptTmpl.Execute(w, input); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	if keeperErr != nil {
		input.Err = keeperErr
		return
	}
	if input.In != "" {
		var in []byte
		if input.Base64 {
			var err error
			in, err = base64.StdEncoding.DecodeString(input.In)
			if err != nil {
				input.Err = fmt.Errorf("Plaintext data was not valid Base64: %v", err)
				return
			}
		} else {
			in = []byte(input.In)
		}
		encrypted, err := keeper.Encrypt(req.Context(), in)
		if err != nil {
			input.Err = fmt.Errorf("Failed to encrypt: %v", err)
			return
		}
		input.Out = base64.StdEncoding.EncodeToString(encrypted)
	}
}

// secretsKeeperDecryptHandler allows the user to enter some data to be decrypted.
func secretsKeeperDecryptHandler(w http.ResponseWriter, req *http.Request) {
	input := &secretsKeeperData{
		URL:    keeperURL,
		In:     req.FormValue("ciphertext"),
		Base64: req.FormValue("base64") == "true",
	}
	defer func() {
		if err := secretsKeeperDecryptTmpl.Execute(w, input); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	if keeperErr != nil {
		input.Err = keeperErr
		return
	}
	if input.In != "" {
		in, err := base64.StdEncoding.DecodeString(input.In)
		if err != nil {
			input.Err = fmt.Errorf("Ciphertext data was not valid Base64: %v", err)
			return
		}
		decrypted, err := keeper.Decrypt(req.Context(), in)
		if err != nil {
			input.Err = fmt.Errorf("Failed to decrypt: %v", err)
			return
		}
		if input.Base64 {
			input.Out = base64.StdEncoding.EncodeToString(decrypted)
		} else {
			input.Out = string(decrypted)
		}
	}
}
