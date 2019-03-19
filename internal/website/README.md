# gocloud.dev source

Source for the [gocloud.dev website][]. Powered by [Hugo][].

[gocloud.dev website]: https://gocloud.dev/
[Hugo]: https://gohugo.io/

## Local Development

Use local hugo server for preview. `cd` into this directory and run:

```
$ hugo server -D
```

This will run the Hugo server that auto-updates its output based on the source
contents. It will print out the `localhost:<PORT>` URL to point the browser to.

This was tested with Hugo 0.53 but should work with subsequent versions as well.

## Editing

Use `hugo new foo/page.md` to create `content/foo/page.md`. This will
automatically add the appropriate [Front Matter][] to the site. After modifying
an existing page, add the `lastmod` attribute with the current ISO date, which
you can obtain with `date -I`. For example:

```yaml
---
title: Foo
date: "2019-03-17T09:00:00-07:00"
lastmod: "2019-03-18T13:30:12-07:00"
---

...
```

[Front Matter]: https://gohugo.io/content-management/front-matter/
