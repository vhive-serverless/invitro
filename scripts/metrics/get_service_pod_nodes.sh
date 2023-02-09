#!/bin/bash

kubectl get pods -A -o custom-columns=NAME:.metadata.name,NODE:.spec.nodeName | grep -v default | tail -n +2 | awk '{print $1","$2}'
