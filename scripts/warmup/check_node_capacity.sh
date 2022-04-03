#!/usr/bin/env bash

kubectl describe node $1 | grep -i capacity -A 13