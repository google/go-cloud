---
title: "MySQL/Postgres"
date: 2019-03-11T10:02:56-07:00
aliases:
- /pages/sql/
---

The `mysql` and `postgres` packages provide functions for connecting to
[MySQL][] and [PostgreSQL][] databases for both on-premise and cloud-provided
instances using the [`*sql.DB`][] type from the Go standard library. Database
connections opened using these packages will automatically collect diagnostic
information via [OpenCensus][].

<!--more-->

Top-level package documentation: https://godoc.org/gocloud.dev/mysql and
https://godoc.org/gocloud.dev/postgres.

[`*sql.DB`]: https://godoc.org/database/sql#DB
[MySQL]: https://www.mysql.com/
[OpenCensus]: https://opencensus.io/
[PostgreSQL]: https://www.postgresql.org/

## Supported Providers

For MySQL:

* [GCP Cloud SQL](https://godoc.org/gocloud.dev/mysql/gcpmysql)
* [AWS RDS](https://godoc.org/gocloud.dev/mysql/awsmysql)
* [Azure Database](https://godoc.org/gocloud.dev/mysql/azuremysql)
* [On-Premise](https://godoc.org/gocloud.dev/mysql) (or locally hosted)

For PostgreSQL:

* [GCP Cloud SQL](https://godoc.org/gocloud.dev/postgres/gcppostgres)
* [AWS RDS](https://godoc.org/gocloud.dev/postgres/awspostgres)
* [On-Premise](https://godoc.org/gocloud.dev/postgres) (or locally hosted)

## Usage Samples

* [Guestbook
  sample](https://github.com/google/go-cloud/tree/master/samples/guestbook)
