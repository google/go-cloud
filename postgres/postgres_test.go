package postgres

import (
	"context"
	"net/url"
	"testing"
)

func TestOpen(t *testing.T) {
	username := "postgres"
	password := ""
	databaseName := "postgres"
	vals := make(url.Values)
	//var urlOpener = URLOpener{}
	var user *url.Userinfo
	vals.Set("sslmode", "disable")

	ctx := context.Background()
	user = url.UserPassword(username, password)
	u := url.URL{
		Scheme:   "postgres",
		User:     user,
		Host:     "localhost",
		Path:     "/" + databaseName,
		RawQuery: vals.Encode(),
	}
	dbByUrl, err := Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := dbByUrl.Ping(); err != nil {
		t.Error("Ping:", err)
	}
	if err := dbByUrl.Close(); err != nil {
		t.Error("Close:", err)
	}
}
