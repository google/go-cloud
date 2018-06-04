# Guestbook Sample

## Building

```shell
# First time, for gowire.
$ vgo vendor

# Now build:
$ gowire && vgo build
```

## Running Locally

```shell
$ docker run --rm --name guestbook-sql -e MYSQL_DATABASE=guestbook -e MYSQL_ROOT_PASSWORD=my-secret -d -p 3306:3306 mysql:5.6
$ cat schema.sql roles.sql | docker run -i --link guestbook-sql:mysql --rm mysql:5.6 \
    sh -c 'exec mysql -h"$MYSQL_PORT_3306_TCP_ADDR" -P"$MYSQL_PORT_3306_TCP_PORT" -uroot -pmy-secret guestbook'
$ ./guestbook -env=local
```

### Teardown

```shell
$ docker stop guestbook-sql
```

## Running on Google Cloud Platform (GCP)

You will also need to modify `inject_gcp.go` with the configuration values you
select.

```shell
$ PROJECT_ID=...
$ GCP_IAM_ACCOUNT=me@example.com
$ GCS_BUCKET_NAME=foo-guestbook-bucket
$ REGION=us-central1
$ RUNTIME_CONFIG_NAME=guestbook
$ RUNTIME_CONFIG_VAR=motd

$ gcloud config set project $PROJECT_ID
$ gcloud config set compute.region $REGION
$ gcloud config set compute.zone ${REGION}-a

$ gcloud iam service-accounts create $GCP_IAM_ACCOUNT
$ gcloud projects add-iam-policy-binding $PROJECT_ID \
    --member=serviceAccount:$GCP_IAM_ACCOUNT \
    --role=roles/cloudsql.client
$ gcloud projects add-iam-policy-binding $PROJECT_ID \
    --member=serviceAccount:$GCP_IAM_ACCOUNT \
    --role=roles/runtimeconfig.admin
$ gcloud projects add-iam-policy-binding $PROJECT_ID \
    --member=serviceAccount:$GCP_IAM_ACCOUNT \
    --role=roles/cloudtrace.agent

$ gcloud sql instances create myinstance \
    --database_version=MYSQL_5_6 \
    --region=$REGION \
    --tier=db-f1-micro \
    --storage-size=10GiB
$ gcloud sql databases create guestbook --instance=myinstance
$ gcloud sql users create guestbook 'cloudsqlproxy~%' --instance=myinstance
$ gcloud sql connect myinstance --user=root < schema.sql

$ gcloud beta runtime-config configs create $RUNTIME_CONFIG_NAME
$ gcloud beta runtime-config configs variables set --config-name=$RUNTIME_CONFIG_NAME --is-text $RUNTIME_CONFIG_VAR "ohai"

$ gsutil mb -c regional -l us-central1 gs://$GCS_BUCKET_NAME
$ gsutil iam ch \
    serviceAccount:${GCP_IAM_ACCOUNT}:objectViewer \
    gs://$GCS_BUCKET_NAME
# On your own: download a GCP icon to gcp.png.
$ gsutil -h 'Content-Type:image/png' cp \
    gcp.png gs://$GCS_BUCKET_NAME/gcp.png

$ gcloud container builds submit -t gcr.io/$PROJECT_ID/guestbook

$ gcloud beta container clusters create mycluster \
    --zone=$ZONE \
    --num-nodes=3 \
    --machine-type=n1-standard-1 \
    --disk-size=100 \
    --addons=HorizontalPodAutoscaling,HttpLoadBalancing,KubernetesDashboard \
    --enable-autorepair \
    --enable-autoupgrade
$ gcloud iam service-accounts keys create gcp-key.json \
    --iam-account=GCP_IAM_ACCOUNT
$ gcloud container clusters get-credentials mycluster
$ kubectl create secret generic guestbook-key --from-file=key.json=gcp-key.json
$ rm gcp-key.json
$ sed -i -e 's|\$GCP_PROJECT_ID|'"$GCP_PROJECT_ID"'|' guestbook-gcp.yaml
$ kubectl apply -f guestbook-gcp.yaml
```

### Teardown

```
$ kubectl delete deployments,services guestbook
$ gcloud beta container clusters delete mycluster
$ gcloud sql instances delete myinstance
$ gcloud iam service-accounts delete $GCP_IAM_ACCOUNT
```

## Running on Amazon Web Services (AWS)

You will also need to modify `inject_aws.go` with the configuration values you
select.

```shell
$ AWS_ACCT_ID="$( aws sts get-caller-identity --query=Account --output=text )"
$ PARAMSTORE_VAR=/guestbook/motd

$ aws configure set default.region us-west-1
$ SGRP_ID="$( aws ec2 create-security-group \
    --group-name=devenv-sg \
    --description="security group for development environment" \
    --query='GroupId' --output=text )"
$ aws ec2 authorize-security-group-ingress \
    --group-name=devenv-sg \
    --protocol=tcp \
    --port=22 \
    --cidr=0.0.0.0/0
$ aws ec2 authorize-security-group-ingress \
    --group-name=devenv-sg \
    --protocol=tcp \
    --port=3306 \
    --cidr=0.0.0.0/0
$ aws ec2 authorize-security-group-ingress \
    --group-name=devenv-sg \
    --protocol=tcp \
    --port=3306 \
    --source-group=devenv-sg
$ aws ec2 authorize-security-group-ingress \
    --group-name=devenv-sg \
    --protocol=tcp \
    --port=8080 \
    --cidr=0.0.0.0/0

$ cat > assume-policy.json <<EOF
{
  "Version": "2012-10-17",
  "Statement": {
    "Effect": "Allow",
    "Principal": {"Service": "ec2.amazonaws.com"},
    "Action": "sts:AssumeRole"
  }
}
EOF
$ aws iam create-role \
    --role-name=guestbook \
    --assume-role-policy-document=file://assume-policy.json
$ cat > role-policy.json <<EOF
{
  "Version": "2012-10-17",
  "Statement": {
    "Effect": "Allow",
    "Action": [
        "s3:GetObject",
        "ssm:DescribeParameters",
        "ssm:GetParameter",
        "ssm:GetParameters",
        "xray:PutTraceSegments",
        "xray:PutTelemetryRecords"
    ],
    "Resource": "*"
  }
}
EOF
$ aws iam put-role-policy \
    --role-name=guestbook \
    --policy-name=Guestbook-Policy \
    --policy-document=file://role-policy.json
$ aws iam create-instance-profile \
    --instance-profile-name=Guestbook
$ aws iam add-role-to-instance-profile \
    --instance-profile-name=Guestbook \
    --role-name=Guestbook

$ aws rds create-db-instance \
    --engine=mysql \
    --engine-version=5.6.39 \
    --db-instance-class=db.t2.micro \
    --allocated-storage=20 \
    --db-instance-identifier=myinstance \
    --master-username=root \
    --master-user-password=xyzzyplugh \
    --vpc-security-group-ids=$SGRP_ID \
    --db-name=guestbook
$ aws rds wait db-instance-available \
    --db-instance-identifier=myinstance
$ RDS_HOST="$( aws rds describe-db-instances \
    --db-instance-identifier=myinstance \
    --query='DBInstances[0].Endpoint.Address'
    --output=text )"
# Copy $RDS_HOST as the endpoint in inject_aws.go.
$ curl -fsSLO https://s3.amazonaws.com/rds-downloads/rds-combined-ca-bundle.pem
$ cat schema.sql roles.sql | mysql \
    -uroot -p \
    -h$RDS_HOST \
    --ssl-ca=rds-combined-ca-bundle.pem \
    --ssl-verify-server-cert
$ aws ec2 revoke-security-group-ingress \
    --group-name=devenv-sg \
    --protocol=tcp \
    --port=3306 \
    --cidr=0.0.0.0/0

$ aws s3 mb s3://$S3_BUCKET_NAME
# On your own: download an AWS icon to aws.png.
$ aws s3 cp --content-type=image/png \
    aws.png s3://$S3_BUCKET_NAME/aws.png

$ aws ec2 create-key-pair \
    --key-name=devenv-key \
    --query=KeyMaterial \
    --output=text > devenv-key.pem
$ INSTANCE_ID="$( aws ec2 run-instances \
    --image-id=ami-364b5856 \
    --security-group-ids=$SGRP_ID \
    --count=1 \
    --instance-type=t2.micro \
    --key-name=devenv-key \
    --iam-instance-profile=Name=Guestbook \
    --query 'Instances[0].InstanceId' --output=text )"
$ aws ec2 wait instance-running --instance-ids=$INSTANCE_ID
$ HOST_ADDR="$( aws ec2 describe-instances \
    --instance-ids=$INSTANCE_ID \
    --query='Reservations[0].Instances[0].PublicIpAddress' \
    --output=text )"

# Optional, to get host key:
$ while [[ "$(aws ec2 get-console-output --instance-id=$INSTANCE_ID --query=Output --output=text)" = None ]]; do sleep 5; done
$ aws ec2 get-console-output \
    --instance-id=$INSTANCE_ID \
    --query=Output \
    --output=text | less
# End optional

$ aws ssm put-parameter --overwrite --name="$PARAMSTORE_VAR" --type=String --value="ohai from AWS"

$ scp -i devenv-key.pem ./guestbook ./aws-key.json admin@${HOST_ADDR}:~
$ echo "URL: http://${HOST_ADDR}:8080/"
$ ssh -i devenv-key.pem admin@$HOST_ADDR

admin@ec2$ AWS_REGION=us-west-1 ./guestbook -env=aws
```

### Introspection

```shell
$ aws ec2 describe-security-groups --query='SecurityGroups[*].{GroupName:GroupName,GroupId:GroupId}'
$ aws rds describe-db-instances \
    --db-instance-identifier=myinstance \
    --query='DBInstances[0].{Endpoint:Endpoint.Address, DbiResourceId:DbiResourceId}'
    --output=text
```

### Teardown

```shell
$ aws ec2 terminate-instances --instance-ids=$INSTANCE_ID
$ aws iam delete-instance-profile --instance-profile-name=guestbook
$ aws ec2 delete-key-pair --key-name=devenv-key
$ aws rds delete-db-instance --db-instance-identifier=myinstance
$ aws ec2 delete-security-group --group-id=$SGRP_ID
```
