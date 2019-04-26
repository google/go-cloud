#!/bin/bash

# Clean up and run Zookeeper.
docker kill zookeeper
docker rm zookeeper
docker run -d --net=host --name=zookeeper -e ZOOKEEPER_CLIENT_PORT=2181 confluentinc/cp-zookeeper:4.1.0

# Clean up and run Kafka.
docker kill kafka
docker rm kafka
docker run --net=host -p 9092:9092 --name=kafka -e KAFKA_ZOOKEEPER_CONNECT=localhost:2181 -e KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://localhost:9092  -e  KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=1 -e KAFKA_AUTO_CREATE_TOPICS_ENABLE=false confluentinc/cp-kafka:4.1.0
