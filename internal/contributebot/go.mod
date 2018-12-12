module github.com/google/go-cloud/internal/contributebot

replace github.com/google/go-cloud => ../..

require (
	cloud.google.com/go v0.30.0
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/google/go-cloud v0.4.0
	github.com/google/go-cmp v0.2.0
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/google/wire v0.2.0
	go.opencensus.io v0.17.0
	golang.org/x/oauth2 v0.0.0-20181017192945-9dcd33a902f4
	google.golang.org/api v0.0.0-20181017004218-3f6e8463aa1d
	google.golang.org/appengine v1.2.0
)
