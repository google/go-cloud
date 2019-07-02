# TODO(rvangent): Add comments explaining.

locals {
  filevar_dir_path = "${path.cwd}/fileblob_scratchdir"
  filevar_url      = "file://${local_file.configfile.filename}?decoder=string"
}

resource "local_file" "configfile" {
  content  = "config value"
  filename = "${local.filevar_dir_path}/config.txt"
}

