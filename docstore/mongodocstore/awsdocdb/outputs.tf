output "setup_ssh_tunnel" {
  value = "ssh -L 27019:${aws_docdb_cluster.docdbtest.endpoint}:27017 ubuntu@${aws_instance.docdbtest.public_dns} -N"
}
