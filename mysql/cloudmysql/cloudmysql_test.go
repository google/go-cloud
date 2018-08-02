package cloudmysql

import "testing"

// TestMakeConfig checks for regression of
// https://github.com/google/go-cloud/issues/275.
func TestMakeConfig(t *testing.T) {
	p := Params{}
	cfg := makeConfig("", &p)
	if !cfg.AllowNativePasswords {
		t.Errorf("cfg.AllowNativePasswords is false")
	}
}
