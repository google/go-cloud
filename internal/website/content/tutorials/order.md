---
title: "Order Processor"
date: 2019-07-09T10:22:39-04:00
draft: true
weight: 5
toc: true
---

In this tutorial, we will run a Go CDK application called Order Processor
on a local machine.

<!--more-->

Order Processor is a sample application that lets users place orders to convert
images to PNG format, and to view the results. The main business logic is
written in a cloud-agnostic manner using the generic APIs for blob, pubsub and
docstore.

The Order Processor application has two parts: a web frontend, and an
image-processing backend called a processor. They communicate over a pubsub
topic, store order information in a docstore collection, and store image files
in a blob bucket.

## Prerequisites

You will need to install the following software for this tutorial:

-   [Git](https://git-scm.com/)
-   [Go](https://golang.org/doc/install)

Then you need to clone the Go CDK repository. The
repository contains the Order Processor sample.

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
The frontend is now running on http://localhost:10538.

Visit the home page in your browser and click "Convert an Image".

Enter an email address (it need not be real) and select any image file from your
computer. Then click Submit.

Now visit the order list page by returning to the home page and clicking "List
Conversions". It make take a few seconds to process the order (thanks to an
artificial delay in the processor), so refresh the page until you see your order
in the list.

Then click on the output image link to see the converted image in your browser.

## Running on a Cloud Provider

To run the Order Processor application on a cloud provider like Google Cloud
Platform, Amazone AWS or Microsoft Azure, you will have to provision
some resources:

- A storage bucket, to hold the image files.
- A Pub/Sub topic and subscription, for requests from the frontend to the
  processor.
- A document store collection (Google Firestore, Amazon DynamoDB or Document DB,
  or Microsoft Cosmos) to store order metadata.

Then launch the `order` program with flags that provide the URLs to your
resources. Run `order -help` to see the list of flags.

