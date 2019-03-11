---
title: "MySQL/Postgres"
---

The `mysql` and `postgres` packages provide functions for connecting to
[MySQL][] and [PostgreSQL][] databases for both on-premise and cloud-provided
instances using the [`*sql.DB`][] type from the Go standard library. Database
connections opened using these packages will automatically collect diagnostic
information via [OpenCensus][].

Top-level package documentation: https://godoc.org/gocloud.dev/mysql and
https://godoc.org/gocloud.dev/postgres.

[`*sql.DB`]: https://godoc.org/database/sql#DB
[MySQL]: https://www.mysql.com/
[OpenCensus]: https://opencensus.io/
[PostgreSQL]: https://www.postgresql.org/

## Supported Providers

For MySQL:

* [GCP Cloud SQL](https://godoc.org/gocloud.dev/mysql/cloudmysql)
* [AWS RDS](https://godoc.org/gocloud.dev/mysql/rdsmysql)
* [On-Premise](https://godoc.org/gocloud.dev/mysql) (or locally hosted)

For PostgreSQL:

* [GCP Cloud SQL](https://godoc.org/gocloud.dev/postgres/cloudpostgres)
* [AWS RDS](https://godoc.org/gocloud.dev/postgres/rdspostgres)
* [On-Premise](https://godoc.org/gocloud.dev/postgres) (or locally hosted)

## Usage Samples

* [Guestbook
  sample](https://github.com/google/go-cloud/tree/master/samples/guestbook)
