# This file specifies Terraform module outputs.
# See https://www.terraform.io/docs/configuration/outputs.html

# The launch environment sets environment variables for the running server
# which can be read inside your program using os.Getenv.
output "launch_environment" {
  value {
    # Example:
    # FOO = "BAR"

    # DO NOT REMOVE THIS COMMENT; GO CDK DEMO URLs WILL BE INSERTED BELOW HERE
  }
}

# The launch specifier sets options for the biome's launcher.
# This is a local launcher, which runs on the local Docker daemon.
output "launch_specifier" {
  value {
    host_port    = 8080
    project_id   = ""            # fill this in
    location     = "us-central1"
    service_name = ""            # fill this in
  }
}
