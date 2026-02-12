#!/bin/bash


go run experiment/khala_command.go --command=deploy
go run experiment/khala_command.go --command=create-snapshots
go run experiment/khala_command.go --command=clean --remove-snapshots=false

go run experiment/khala_command.go --command=deploy --set-nexus-sdk=true --set-nexus-rpc=true
go run experiment/khala_command.go --command=create-snapshots --set-nexus-sdk=true --set-nexus-rpc=true
go run experiment/khala_command.go --command=clean --remove-snapshots=false

sleep 60