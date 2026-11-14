#!/bin/bash
echo "START"

cleanup() {
  echo "===== Cleanup ====="
  kubectl get ksvc -n default -o name | grep 'trace-func-' | xargs -r kubectl delete -n default --wait=true
  # wait until the trace-func pods are gone, so the next run starts from an empty cluster
  while kubectl get pods -n default --no-headers 2>/dev/null | grep -q 'trace-func-'; do sleep 5; done
}

cleanup

echo "===== TEST 4 ====="

# Variables
## Azure2019Expo Vars
AZURE2019EXPO_JSON_FILE="config_fyp_test_4_2019_expo.json"
AZURE2019EXPO_DIRECTORY="test_4_2019_expo"
mkdir -p "data/out/$AZURE2019EXPO_DIRECTORY"
mkdir -p "data/out/$AZURE2019EXPO_DIRECTORY/iat"

## Azure2019Shift Vars
AZURE2019SHIFT_JSON_FILE="config_fyp_test_4_2019_shift.json"
AZURE2019SHIFT_DIRECTORY="test_4_2019_shift"
mkdir -p "data/out/$AZURE2019SHIFT_DIRECTORY"
mkdir -p "data/out/$AZURE2019SHIFT_DIRECTORY/iat"

## Azure2021 Vars
AZURE2021_JSON_FILE="config_fyp_test_4_2021.json"
AZURE2021_DIRECTORY="test_4_2021"
mkdir -p "data/out/$AZURE2021_DIRECTORY"
mkdir -p "data/out/$AZURE2021_DIRECTORY/iat"

# Azure2019 Expo
echo "===== Azure2019 Expo ====="
go run cmd/loader.go --config "cmd/$AZURE2019EXPO_JSON_FILE" --verbosity=trace 2>&1 | tee Azure2019_expo_output.txt
cleanup
mv Azure2019_expo_output.txt "data/out/$AZURE2019EXPO_DIRECTORY"

echo "===== Azure2019 Expo - IAT Generation ====="
go run cmd/loader.go --config "cmd/$AZURE2019EXPO_JSON_FILE" --verbosity=trace --iatGeneration=true
mv *.json "data/out/$AZURE2019EXPO_DIRECTORY/iat"

# Azure2019 Shift
echo "===== Azure2019 Shift ====="
go run cmd/loader.go --config "cmd/$AZURE2019SHIFT_JSON_FILE" --verbosity=trace 2>&1 | tee Azure2019_shift_output.txt
cleanup
mv Azure2019_shift_output.txt "data/out/$AZURE2019SHIFT_DIRECTORY"

echo "===== Azure2019 Shift - IAT Generation ====="
go run cmd/loader.go --config "cmd/$AZURE2019SHIFT_JSON_FILE" --verbosity=trace --iatGeneration=true
mv *.json "data/out/$AZURE2019SHIFT_DIRECTORY/iat"


# Azure2021
echo "===== Azure2021 ====="
go run cmd/loader.go --config "cmd/$AZURE2021_JSON_FILE" --verbosity=trace 2>&1 | tee Azure2021_output.txt
cleanup
mv Azure2021_output.txt "data/out/$AZURE2021_DIRECTORY"

echo "===== Azure2021 - IAT Generation ====="
go run cmd/loader.go --config "cmd/$AZURE2021_JSON_FILE" --verbosity=trace --iatGeneration=true
mv *.json "data/out/$AZURE2021_DIRECTORY/iat"

# Upload Dataset
echo "===== Uploading data to Google Drive ====="
rclone sync ~/loader/data/out remote:RClone/test_3 --progress

echo "END"



