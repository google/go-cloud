package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"time"

	"gocloud.dev/secrets"
	_ "gocloud.dev/secrets/awskms"
	_ "gocloud.dev/secrets/azurekeyvault"
	_ "gocloud.dev/secrets/gcpkms"
	_ "gocloud.dev/secrets/hashivault"
	_ "gocloud.dev/secrets/localsecrets"
)

// TODO(rvangent): This file is user-visible, add many comments explaining
// how it works.

func init() {
	http.HandleFunc("/demo/secrets/", secretsEncryptHandler)
	http.HandleFunc("/demo/secrets/encrypt", secretsEncryptHandler)
	http.HandleFunc("/demo/secrets/decrypt", secretsDecryptHandler)
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	keeper, keeperErr = secrets.OpenKeeper(ctx, keeperURL)
}

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
    This page demonstrates the use of Go CDK's <a href="https://godoc.org/gocloud.dev/secrets">secrets</a> package.
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
    <li><a href="./encrypt">Encrypt</a> using the secrets.Keeper</li>
    <li><a href="./decrypt">Decrypt</a> using the secrets.Keeper</li>
  </ul>
  {{if .Err}}
    <p><strong>{{ .Err }}</strong></p>
  {{end}}`

	secretsTemplateSuffix = `
</body>
</html>`

	// Input: *secretsData.
	secretsEncryptTemplate = secretsTemplatePrefix + `
  <form>
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
      <a href="./decrypt?ciphertext={{ .Out }}&base64={{ .Base64 }}">Decrypt it</a>
    </div>
  {{end}}` + secretsTemplateSuffix

	// Input: *secretsData.
	secretsDecryptTemplate = secretsTemplatePrefix + `
  <form>
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
      <a href="./encrypt?plaintext={{ .Out }}&base64={{.Base64}}">Encrypt it</a>
    </div>
  {{end}}` + secretsTemplateSuffix
)

var (
	secretsEncryptTmpl = template.Must(template.New("secrets encrypt").Parse(secretsEncryptTemplate))
	secretsDecryptTmpl = template.Must(template.New("secrets decrypt").Parse(secretsDecryptTemplate))
)

// secretsEncryptHandler allows the user to enter some data to be encrypted.
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

	if keeperErr != nil {
		data.Err = keeperErr
		return
	}
	if data.In != "" {
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
		encrypted, err := keeper.Encrypt(req.Context(), in)
		if err != nil {
			data.Err = fmt.Errorf("Failed to encrypt: %v", err)
			return
		}
		data.Out = base64.StdEncoding.EncodeToString(encrypted)
	}
}

// secretsDecryptHandler allows the user to enter some data to be decrypted.
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

	if keeperErr != nil {
		data.Err = keeperErr
		return
	}
	if data.In != "" {
		in, err := base64.StdEncoding.DecodeString(data.In)
		if err != nil {
			data.Err = fmt.Errorf("Ciphertext data was not valid Base64: %v", err)
			return
		}
		decrypted, err := keeper.Decrypt(req.Context(), in)
		if err != nil {
			data.Err = fmt.Errorf("Failed to decrypt: %v", err)
			return
		}
		if data.Base64 {
			data.Out = base64.StdEncoding.EncodeToString(decrypted)
		} else {
			data.Out = string(decrypted)
		}
	}
}
