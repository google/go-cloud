package replay

import (
	"testing"
)

func TestJSONScrub(t *testing.T) {
	const key = "LastModifiedUser"
	var tests = []struct {
		body, want string
	}{
		{
			body: `'{"Parameters":[{"LastModifiedDate":1.529008678885E9,"LastModifiedUser":"arn:aws:iam::1234:user/TESTUSER","Name":"test","Policies":[],"Type":"String","Version":1}]}'`,
			want: `'{"Parameters":[{"LastModifiedDate":1.529008678885E9,"Name":"test","Policies":[],"Type":"String","Version":1}]}'`,
		},
		{
			body: `'{"Parameters":[{"LastModifiedDate":1.529008678885E9,"LastModified":"arn:aws:iam::1234:user/TESTUSER","Name":"test","Policies":[],"Type":"String","Version":1}]}'`,
			want: `'{"Parameters":[{"LastModifiedDate":1.529008678885E9,"LastModified":"arn:aws:iam::1234:user/TESTUSER","Name":"test","Policies":[],"Type":"String","Version":1}]}'`,
		},
		{
			body: "",
			want: "",
		},
		{
			body: "picard",
			want: "picard",
		},
		{
			body: "picardLastModifiedUser:riker",
			want: "picardLastModifiedUser:riker",
		},
	}

	for _, tc := range tests {
		t.Run("", func(t *testing.T) {
			if got := removeJSONString(tc.body, key); got != tc.want {
				t.Errorf("got %v; want %v", got, tc.want)
			}
		})
	}
}
