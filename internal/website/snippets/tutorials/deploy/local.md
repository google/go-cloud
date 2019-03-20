## Clone the Guestbook Sample

First, download the Go CDK repository:

```shell
git clone https://github.com/google/go-cloud.git
cd go-cloud/samples/guestbook
```

## Building

Run the following in the `samples/guestbook` directory:

```shell
go generate && go build
```

## Running Locally

You will need to run a local MySQL database server and create a local message of
the day. `localdb/main.go` runs the local MySQL database server using Docker:

```shell
go get ./localdb/... # Get package dependencies.
go run localdb/main.go
```

In another terminal, run the `guestbook` application:

```shell
# Set a local Message of the Day.
echo 'Hello, World!' > motd.txt

# Run the server.
# For blob, it uses fileblob, pointing at the local directory ./blobs.
# For runtimevar, it uses filevar, pointing at the local file ./motd.txt.
#   You can update the ./motd.txt while the server is running, refresh
#   the page, and see it change.
./guestbook -env=local -bucket=blobs -motd_var=motd.txt
```

Your server is now running on http://localhost:8080/.

You can stop the MySQL database server with Ctrl-\\. MySQL ignores Ctrl-C
(SIGINT).
