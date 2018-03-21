#!/usr/bin/env bash

# Copyright 2018 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

. prepare_common.bash

EPOCH=1
REGION=$(aws configure get region)

#############
# Amazon S3 #
#############
BUCKET="go-cloud-demo"
BLOBNAME="go-cloud-demo-aws-hello.txt" # already created, world-readable
BLOBURI="s3://$BUCKET/$BLOBNAME"

############################
# Amazon DB Security Group #
############################
DBSECURITYGROUP=go-cloud-demo-public
if aws rds describe-db-security-groups --output text --query "DBSecurityGroups[?DBSecurityGroupName==\`$DBSECURITYGROUP\`].DBSecurityGroupName" | grep $DBSECURITYGROUP > /dev/null; then
  log "AWS RDS DB security group: $DBSECURITYGROUP (existing)"
else
  log "AWS RDS DB security group: $DBSECURITYGROUP (new)"
  aws rds create-db-security-group --db-security-group-name $DBSECURITYGROUP --db-security-group-description "$DBSECURITYGROUP"
  aws rds authorize-db-security-group-ingress --db-security-group-name $DBSECURITYGROUP --cidrip 0.0.0.0/0
fi

####################
# Amazon RDS MySQL #
####################
DBINSTANCE=go-cloud-demo-db-instance$EPOCH
DBUSER=root
DBPASSWORD=db-cloud-demo-password
DBNAME=go-cloud-demo-db
# $RANDOM$RANDOM # IAM authentication is used
if aws rds describe-db-instances --output=text --query="DBInstances[?DBInstanceIdentifier==\`$DBINSTANCE\`].DBInstanceIdentifier" | grep $DBINSTANCE > /dev/null; then
  log "AWS RDS Instance: $DBINSTANCE (existing)"
else
  log "AWS RDS Instance: $DBINSTANCE (new)."
  aws rds create-db-instance \
    --db-instance-identifier $DBINSTANCE \
    --db-instance-class db.m3.medium \
    --engine MySQL \
    --allocated-storage 20 \
    --master-username $DBUSER \
    --master-user-password $DBPASSWORD \
    --publicly-accessible \
    --db-security-groups $DBSECURITYGROUP \
    --enable-iam-database-authentication > /dev/null
  log "Waiting for database to become available. This may take a long time. You can monitor progress at:"
  log "https://console.aws.amazon.com/rds/home?region=$REGION#dbinstances:"
  aws rds wait db-instance-available \
    --db-instance-identifier $DBINSTANCE
fi

DBHOSTNAME=$(aws rds describe-db-instances --output=text --query="DBInstances[?DBInstanceIdentifier==\`$DBINSTANCE\`].Endpoint.Address")
mysql --host=$DBHOSTNAME \
  --port=3306 \
  --ssl-ca=$(pwd)/rds-combined-ca-bundle.pem \
  --ssl-verify-server-cert \
  --user=root \
  --password=$DBPASSWORD <<EOF
DROP DATABASE IF EXISTS \`$DBNAME\`;
CREATE DATABASE \`$DBNAME\`;
USE \`$DBNAME\`;
CREATE TABLE blobs (
name VARCHAR(255),
blob_name VARCHAR(255)
);
INSERT INTO blobs (name, blob_name) VALUES ('hellomessage', '$BLOBURI');
quit
EOF

DBURI="mysql://root:$DBPASSWORD@tcp($DBHOSTNAME)/$DBNAME" # ?tls=True

##########################################
# Amazon Simple Systems Manager (config) #
##########################################
CONFIGNAME=go-cloud-demo-config
PARAMETERNAME=go-cloud-demo-db
parts=("$CONFIGNAME" "$PARAMETERNAME")
printf -v CONFIGPATH '/%s' "${parts[@]%/}"
aws ssm put-parameter --name $CONFIGPATH --type String --value "$DBURI" --overwrite

###################################
# Start application (for testing) #
###################################
log "Starting server"
go run -tags 'aws' example.go -config=ssm://$CONFIGNAME
