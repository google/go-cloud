---
title: "mysql"
date: 2019-03-11T10:02:56-07:00
aliases:
- /ref/sql/
- /pages/sql/
---

The `mysql` package provides functions for connecting to [MySQL][] databases
for both on-premise and cloud-provided instances using the [`*sql.DB`][] type
from the Go standard library. Database connections opened using this package
will automatically collect diagnostic information via [OpenCensus][].

<!--more-->

[How-to guide]({{< ref "/howto/sql/_index.md" >}})<br>
[Top-level package documentation](https://godoc.org/gocloud.dev/mysql)

[`*sql.DB`]: https://godoc.org/database/sql#DB
[MySQL]: https://www.mysql.com/
[OpenCensus]: https://opencensus.io/

## Supported Services

* [GCP Cloud SQL for MySQL](https://godoc.org/gocloud.dev/mysql/gcpmysql)
* [AWS RDS for MySQL](https://godoc.org/gocloud.dev/mysql/awsmysql)
* [Azure Database for MySQL](https://godoc.org/gocloud.dev/mysql/azuremysql)
* [On-Premise](https://godoc.org/gocloud.dev/mysql) (or locally hosted)

## Usage Samples

* [Guestbook sample](https://gocloud.dev/tutorials/guestbook/)
