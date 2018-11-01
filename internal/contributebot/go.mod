module github.com/google/go-cloud/internal/contributebot

replace github.com/google/go-cloud => ../..

require (
	cloud.google.com/go v0.30.0
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/google/go-cloud v0.4.0
	github.com/google/go-cmp v0.2.0
	github.com/google/go-github/v18 v18.2.0
	go.opencensus.io v0.17.0
	golang.org/x/net v0.0.0-20181029044818-c44066c5c816 // indirect
	golang.org/x/oauth2 v0.0.0-20181031022657-8527f56f7107
	golang.org/x/sys v0.0.0-20181031143558-9b800f95dbbc // indirect
	google.golang.org/api v0.0.0-20181017004218-3f6e8463aa1d
	google.golang.org/appengine v1.2.0
)
