#!/bin/bash
echo "START"

cleanup() {
  echo "===== Cleanup ====="
  kubectl get ksvc -n default -o name | grep 'trace-func-' | xargs -r kubectl delete -n default --wait=true
  # wait until the trace-func pods are gone, so the next run starts from an empty cluster
  while kubectl get pods -n default --no-headers 2>/dev/null | grep -q 'trace-func-'; do sleep 5; done
}

echo "==== Cleanup ===="
cleanup

echo "===== TEST 2 ====="

# Azure2019 Expo
echo "===== Azure2019 Expo ====="
go run cmd/loader.go --config cmd/config_fyp_test_2_2019_expo.json --verbosity=trace 2>&1 | tee Azure2019_expo_output.txt
cleanup
mv Azure2019_expo_output.txt data/out/test_2_2019_expo

echo "===== Azure2019 Expo - IAT Generation ====="
go run cmd/loader.go --config cmd/config_fyp_test_2_2019_expo.json --verbosity=trace --iatGeneration=true
mkdir -p data/out/test_2_2019_expo/iat
mv *.json data/out/test_2_2019_expo/iat


# Azure2019 Shift
echo "===== Azure2019 Shift ====="
go run cmd/loader.go --config cmd/config_fyp_test_2_2019_shift.json --verbosity=trace 2>&1 | tee Azure2019_shift_output.txt
cleanup
mv Azure2019_shift_output.txt data/out/test_2_2019_shift

echo "===== Azure2019 Shift - IAT Generation ====="
go run cmd/loader.go --config cmd/config_fyp_test_2_2019_shift.json --verbosity=trace --iatGeneration=true
mkdir -p data/out/test_2_2019_shift/iat
mv *.json data/out/test_2_2019_shift/iat


# Azure2021
echo "===== Azure2021 ====="
go run cmd/loader.go --config cmd/config_fyp_test_2_2021.json --verbosity=trace 2>&1 | tee Azure2021_output.txt
cleanup
mv Azure2021_output.txt data/out/test_2_2021


echo "===== Azure2021 - IAT Generation ====="
go run cmd/loader.go --config cmd/config_fyp_test_2_2021.json --verbosity=trace --iatGeneration=true
mkdir -p data/out/test_2_2021/iat
mv *.json data/out/test_2_2021/iat

echo "===== Uploading data to Google Drive ====="
rclone sync ~/loader/data/out remote:RClone/test_2 --progress

echo "END"



