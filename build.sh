#!/bin/bash
CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .
docker build -t k8s_namespace_manager .
