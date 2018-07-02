# Getting Started with Go X Cloud

The best way to understand Go X Cloud is to write some code and use it. To do that,
let's start with something simple. Blob storage is a well-understood concept and
is one of the most frequently used cloud services. So let's build a command line
application that uploads files to blog storage on both AWS and GCP. We'll call
it `upload`.

Before we start with any code, it helps to think about possible designs. If a
product manager asked us to build an upload tool, we could very well write a
code path for Simple Storage Service (S3) and another code path for Google Cloud
Storage (GCS). And that would work. However, it would also be tedious. We would
have to learn the semantics of uploading files to both blob storage services.
And, even worse, we would have two code paths that effectively do the same
thing, but would have to be maintained separately (ugh!). It would be much nicer
if we could write the upload logic once and reuse it across providers. That's
exactly what Go X Cloud makes possible. So, let's write some code!

When we're done, our command line application will work like this:

``` shell
# uploads gopher.png to GCS
$ ./upload -cloud gcp -file ./gopher.png

# uploads gopher.png to S3
$ ./upload -cloud aws -file ./gopher.png
```

We start with a skeleton for our program with flags to configure the cloud to
use and the path of the file to upload.

``` go
package main

import "flag"

func main() {
    // define our input flags
    cloud := flag.String("cloud", "", "Cloud storage to use")
    file := flag.String("file", "", "Path to the file to upload")
    flag.Parse()

    // TODO:
    // open a connection to the bucket
    // prepare the file for upload
    // write to the bucket
    // close the connection
}
```

Now that we have a basic skeleton in place, let's start filling in the pieces.
Opening a connection to the bucket will depend on which cloud we're using.
Because we allow the user to specify a cloud, we will write connection logic for
both clouds. This will be the only step that requires cloud-specific APIs.

As a first pass, let's connect to a GCS bucket. Then, we will connect to an S3
bucket. In both cases, we are going to create a pointer to a `blob.Bucket`, the
type Go X Cloud provides as the cloud-agnostic interface into blob storage across
clouds.

``` go
package main

import (
    "context"
    "flag"
    "log"

    "github.com/google/go-x-cloud/blob"
    "github.com/google/go-x-cloud/blob/gcsblob"
    "github.com/google/go-x-cloud/gcp"
)

func main() {
    // flag setup omitted
    // ...

    // open a connection to the bucket
    var (
        b *blob.Bucket
        err error
    )
    switch *cloud {
    case "gcp":
        // gcp.DefaultCredentials assumes GOOGLE_APPLICATION_CREDENTIALS is present
        // in tne environment and points to a service-account.json file.
        creds, err := gcp.DefaultCredentials(context.Background())
        if err != nil {
            log.Fatalf("Failed to create default credentials for GCP: %s", err)
        }
        c, err := gcp.NewHTTPClient(gcp.DefaultTransport(), gcp.CredentialsTokenSource(creds))
        if err != nil {
            log.Fatalf("Failed to create HTTP client: %s", err)
        }
        b, err = gcsblob.NewBucket(context.Background(), "my-cool-bucket", c)
        if err != nil {
            log.Fatalf("Failed to connect to bucket: %s", err)
        }
    case "aws":
        // connect to s3 bucket
        // ...
    default:
        log.Fatalf("Failed to recognize cloud. Want gcp or aws, got: %s", *cloud)
    }

    // TODO:
    // prepare the file for upload
    // write to the bucket
    // close the connection
}
```

The cloud-specific setup is the tricky part as each cloud has its own series of
invocations to create a bucket.

Let's handle the S3 connection logic next:

``` go
package main

import (
    // ...
    // listing only new import statements

    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/aws/credentials"
    "github.com/aws/aws-sdk-go/aws/session"
)


func main() {
    // ...
    switch *cloud {
    case "gcp":
        // ...
    case "aws":
        c := &aws.Config{
            Region: aws.String("us-east-2"), // or wherever the bucket is
            // credentials.NewEnvCredentials assumes two environment variables are
            // present:
            // 1. AWS_ACCESS_KEY_ID, and
            // 2. AWS_SECRET_ACCESS_KEY.
            Credentials: credentials.NewEnvCredentials(),
        }
        s := session.Must(session.NewSession(c))
        b, err = s3blob.NewBucket(context.Background(), s, "my-cool-bucket")
        if err != nil {
            log.Fatalf("Failed to connect to S3 bucket: %s", err)
        }
    default:
        // ...
    }

    // TODO:
    // prepare the file for upload
    // write to the bucket
    // close the connection
}
```

The important point here is that in spite of the differences in setup for GCS
and S3, the result is the same: we have a pointer to a `blob.Bucket`. It's hard
to understate how powerful it is to have one type that can be used across
clouds. Provided we go through the setup, once we have a `blob.Bucket` pointer,
we may design our system around that type and prevent any cloud specific
concerns from leaking throughout our application code. In other words, by using
`blob.Bucket` we avoid being tightly coupled to one cloud.

With the setup done, we're ready to use the bucket connection. Note, as a design
principle of Go X Cloud, `blob.Bucket` does not support creating a bucket and
instead focuses solely on interacting with it. This separates the concerns of
provisioning infrastructure and using infrastructure.

We need to convert our file into a slice of bytes before uploading it. The
process is the usual one:

``` go
package main

import (
    // previous imports omitted
    "os"
    "ioutil"
)

func main() {
    // ... previous code omitted

    // prepare the file for upload
    f, err := os.Open(*file)
    if err != nil {
        log.Fatalf("Failed to open file: %s", err)
    }
    data, err := ioutil.ReadAll(f)
    if err != nil {
        log.Fatalf("Failed to read file: %s", err)
    }
    if err = f.Close(); err != nil {
        log.Fatalf("Failed to close file: %s", err)
    }

    // TODO:
    // write to the bucket
    // close the connection
}
```

Now, we have `data`, our file in a slice of bytes. Let's get to the fun part and
write those bytes to the bucket!

``` go
package main

// no new imports

func main() {
    // ...

    w, err := b.NewWriter(context.Background(), "gopher", &blob.WriterOptions{
        // png is hard coded here for brevity to avoid dealing with mime type
        // parsing.
        ContentType: "image/png",
    })
    if err != nil {
        log.Fatalf("Failed to obtain writer: %s", err)
    }
    _, err = w.Write(data)
    if err != nil {
        log.Fatalf("Failed to write to bucket: %s", err)
    }
    if err := w.Close(); err != nil {
        log.Fatalf("Failed to close: %s", err)
    }
}
```

First, we create a write based on the bucket connection. In addition to a
`context.Context` type, the method takes the key underwich the data will be
store and the mime type of the data. For brevity's sake, I have hard-coded
`image/png`, but adding mime type detection isn't difficult.

The call to `NewWriter` creates a `blob.Writer`, which is similar to the
well-known `io.Writer`. With the writer, we simply call `Write` passing in the
data. In response, we get the number of bytes written and any error that might
have occurred.

Finally, we close the writer with `Close` and check the error.

And that's it! Let's try it out. As setup, we will to create an S3 bucket and a
GCS bucket. In the code above, I called that bucket `my-cool-bucket`, but you
can change that to match whatever your bucket is called. For GCP, you will need
a service account key. See
[here](https://cloud.google.com/iam/docs/creating-managing-service-account-keys)
for details. For AWS, you will need an access key ID and a secret access key.
See
[here](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_access-keys.html#Using_CreateAccessKey)
for details.

With our buckets created in S3 and GCS and our credentials for both set up,
we'll build the program first:

``` shell
$ go build -o upload
```

Then, we will send `gopher.png` (in the same directory as this README) to GCS:

``` shell
$ ./upload -cloud gcp -file gopher.png
```

And then, we send that same gopher to S3:

``` shell
$ ./upload -cloud aws -file gopher.png
```

And if we check both buckets, we should see our gopher in both places! We're
done!

In conclusion, we have a program that can seamlessly switch between GCS and S3
using just one code path. Granted, this example isn't complex, but I hope it
demonstrates how even for small programs, having one type for multiple clouds is
a huge win for simplicity and maintainability. By writing an application using a
generic interface like `*blob.Bucket`, we retain the option of using
infrastructure in whichever cloud best fits our needs all without having to
worry about a rewrite.
