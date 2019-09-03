module gocloud.dev/samples

require (
	contrib.go.opencensus.io/exporter/stackdriver v0.12.1
	github.com/Azure/azure-pipeline-go v0.1.9
	github.com/Azure/azure-storage-blob-go v0.6.0
	github.com/aws/aws-sdk-go v1.19.45
	github.com/go-sql-driver/mysql v1.4.1
	github.com/google/go-cmdtest v0.0.0-20190903171304-6f1d2cce5266
	github.com/google/go-cmp v0.3.1
	github.com/google/subcommands v1.0.1
	github.com/google/uuid v1.1.1
	github.com/google/wire v0.3.0
	github.com/gorilla/mux v1.7.2
	github.com/streadway/amqp v0.0.0-20190827072141-edfb9018d271
	go.opencensus.io v0.22.0
	gocloud.dev v0.16.0
	gocloud.dev/docstore/mongodocstore v0.16.0
	gocloud.dev/pubsub/kafkapubsub v0.16.0
	gocloud.dev/pubsub/natspubsub v0.16.0
	gocloud.dev/pubsub/rabbitpubsub v0.16.0
	gocloud.dev/runtimevar/etcdvar v0.16.0
	gocloud.dev/secrets/hashivault v0.16.0
	google.golang.org/genproto v0.0.0-20190620144150-6af8c5fc6601
	gopkg.in/pipe.v2 v2.0.0-20140414041502-3c2ca4d52544
)

replace gocloud.dev => ../

replace gocloud.dev/docstore/mongodocstore => ../docstore/mongodocstore

replace gocloud.dev/pubsub/kafkapubsub => ../pubsub/kafkapubsub

replace gocloud.dev/pubsub/natspubsub => ../pubsub/natspubsub

replace gocloud.dev/pubsub/rabbitpubsub => ../pubsub/rabbitpubsub

replace gocloud.dev/runtimevar/etcdvar => ../runtimevar/etcdvar

replace gocloud.dev/secrets/hashivault => ../secrets/hashivault
