#!/usr/bin/env bash

#* Result line i: <CPU of container user-container i> <Memory of container user-container i>
kubectl top pod --containers | grep user-container | awk '{print $3,$4}'
