#!/usr/bin/env bash

kubectl get po | grep $1 | wc -l