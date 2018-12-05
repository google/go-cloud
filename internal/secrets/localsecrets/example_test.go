// Copyright 2018 The Go Cloud Authors
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

package localsecrets_test

import (
	"context"
	"fmt"
	"github.com/google/go-cloud/internal/secrets/localsecrets"
)

func ExampleEncrypterDecrypterEncrypt() {
	secretKey := "I'm a secret string!"
	skr := localsecrets.NewSecretKeeper(sk)
	e := skr.Encrypter
	d := skr.Decrypter

	msg := "I'm a message!"
	encryptedMsg, err := e.Encrypt(context.Background(), []byte(msg))
	if err != nil {
		panic(err)
	}

	decryptedMsg, err := d.Decrypt(context.Background(), encryptedMsg)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(decryptedMsg))

	// Output:
	// I'm a message!
}
