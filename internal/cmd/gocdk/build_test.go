package main

import (
	"testing"
)

func TestParseImageNameFromDockerfile(t *testing.T) {
	tests := []struct {
		name       string
		dockerfile string
		want       string
		wantErr    bool
	}{
		{
			name:       "Empty",
			dockerfile: "",
			wantErr:    true,
		},
		{
			name:       "NoComment",
			dockerfile: "FROM scratch\n",
			wantErr:    true,
		},
		{
			name: "CommentAtStart",
			dockerfile: "# gocdk-image: hello-world\n" +
				"FROM scratch\n",
			want: "hello-world",
		},
		{
			name: "CommentNoEOL",
			dockerfile: "FROM scratch\n" +
				"# gocdk-image: hello-world",
			want: "hello-world",
		},
		{
			name: "CommentLastLine",
			dockerfile: "FROM scratch\n" +
				"# gocdk-image: hello-world\n",
			want: "hello-world",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseImageNameFromDockerfile([]byte(test.dockerfile))
			if err != nil {
				if !test.wantErr {
					t.Errorf("unexpected error: %+v", err)
				}
				return
			}
			if got != test.want {
				t.Errorf("parseImageNameFromDockerfile(%q) = %q; want %q", test.dockerfile, got, test.want)
			}
		})
	}
}
