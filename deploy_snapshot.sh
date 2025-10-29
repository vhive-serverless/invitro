#!/bin/bash


go run experiment/khala_command.go --command=deploy
go run experiment/khala_command.go --command=create-snapshots
go run experiment/khala_command.go --command=clean --remove-snapshots=false

sleep 60