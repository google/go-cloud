---
title: "Secrets"
date: 2019-03-21T17:42:18-07:00
showInSidenav: true
---

Cloud applications frequently need to store sensitive information like web
API credentials or encryption keys in a medium that is not fully secure. For
example, an application that interacts with GitHub needs to store its OAuth2
client secret and use it when obtaining end-user credentials. If this
information was compromised, it could allow someone else to impersonate the
application. In order to keep such information secret and secure, you can
encrypt the data, but then you need to worry about rotating the encryption
keys and distributing them securely to all of your application servers.
Most cloud providers include a key management service to perform these tasks,
usually with hardware-level security and audit logging.

<!--more-->

The Go CDK provides access to key management providers in a portable way
called "secret keepers". These guides show how to work with secret keepers in
the Go CDK.

