module gocloud.dev/internal/cmd/gocdk

go 1.12

require (
	github.com/google/go-cmp v0.3.0
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/shurcooL/httpfs v0.0.0-20190527155220-6a4d4a70508b // indirect
	github.com/shurcooL/vfsgen v0.0.0-20181202132449-6a9ea43bcacd
	github.com/spf13/cobra v0.0.3
	github.com/spf13/pflag v1.0.3 // indirect
	gocloud.dev v0.15.0
	golang.org/x/oauth2 v0.0.0-20190523182746-aaccbc9213b0
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	golang.org/x/sys v0.0.0-20190530182044-ad28b68e88f1
	golang.org/x/xerrors v0.0.0-20190513163551-3ee3066db522
	google.golang.org/api v0.5.0
)

replace gocloud.dev => ../../../
