---
title: "Order"
date: 2019-07-09T10:22:39-04:00
draft: true
showInSidenav: false  # only for sections (any level)
pagesInSidenav: false  # only for top-level sections
weight: 5
---

In this tutorial, we will run a Go CDK application called Order
on a local machine.

<!--more-->

Order is a sample application that lets users place orders to convert images to
PNG format, and to view the results. The main business logic is written in a
cloud-agnostic manner using the generic APIs for blob, pubsub and docstore.

The Order application has two parts: a web frontend, and an image-processing
backend called a processor. They communicate over a pubsub topic, store order
information in a docstore collection, and store image files in a blob bucket.

## Prerequisites

You will need to install the following software for this tutorial:

-   [Git](https://git-scm.com/)
-   [Go](https://golang.org/doc/install)

Then you need to clone the Go CDK repository. The
repository contains the Order sample.

```shell
git clone https://github.com/google/go-cloud.git
cd go-cloud/samples/order
```

## Building

Run the following in the `samples/order` directory:

```shell
go build
```

## Running Locally


If you run `order` with no arguments, both the frontend and the processor will
run together in the same process. 

```shell
./order
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

