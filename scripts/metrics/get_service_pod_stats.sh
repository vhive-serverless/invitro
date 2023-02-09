#!/bin/bash

kubectl top pods -A | grep -v default | tail -n +3 | awk '{print $2","$3}'
