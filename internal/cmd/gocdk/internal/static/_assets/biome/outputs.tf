# This file specifies Terraform module outputs.
# See https://www.terraform.io/docs/configuration/outputs.html

# The launch environment sets environment variables for the running server
# which can be read inside your program using os.Getenv.
output "launch_environment" {
  value = {
    # Example:
    # FOO = "BAR"

    # DO NOT REMOVE THIS COMMENT; GO CDK DEMO URLs WILL BE INSERTED BELOW HERE
  }
}

# The launch specifier sets options for the biome's launcher.
output "launch_specifier" {
  value = {
    {{ range . -}}
    {{ .Key }} = {{ .Value }}
    {{ end -}}
  }
}
