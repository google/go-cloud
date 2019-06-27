---
title: "postgres"
date: 2019-06-27T13:03:45-07:00
---

The `postgres` package provides functions for connecting to [PostgreSQL][]
databases for both on-premise and cloud-provided instances using the
[`*sql.DB`][] type from the Go standard library. Database connections opened
using these packages will automatically collect diagnostic information via
[OpenCensus][].

<!--more-->

Top-level package documentation: https://godoc.org/gocloud.dev/postgres.

[`*sql.DB`]: https://godoc.org/database/sql#DB
[OpenCensus]: https://opencensus.io/
[PostgreSQL]: https://www.postgresql.org/

## Supported Providers

* [GCP Cloud SQL for PostgreSQL](https://godoc.org/gocloud.dev/postgres/gcppostgres)
* [AWS RDS for PostgreSQL](https://godoc.org/gocloud.dev/postgres/awspostgres)
* [On-Premise](https://godoc.org/gocloud.dev/postgres) (or locally hosted)
