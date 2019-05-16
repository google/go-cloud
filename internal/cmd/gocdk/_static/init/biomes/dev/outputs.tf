# This file specifies Terraform module outputs.
# See https://www.terraform.io/docs/configuration/outputs.html

# The launch specifier sets options for the biome's launcher.
# This is a local launcher, which runs on the local Docker daemon.
output "launch_specifier" {
	value {
		host_port = 8080
	}
}
