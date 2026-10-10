#!/bin/bash
echo "START"

cleanup() {
  echo "===== Cleanup ====="
  kubectl get ksvc -n default -o name | grep 'trace-func-' | xargs -r kubectl delete -n default --wait=true
  # wait until the trace-func pods are gone, so the next run starts from an empty cluster
  while kubectl get pods -n default --no-headers 2>/dev/null | grep -q 'trace-func-'; do sleep 5; done
}

echo "===== TEST 1 ====="

# Azure2019 Expo
echo "===== Azure2019 Expo ====="
go run cmd/loader.go --config cmd/config_fyp_test_1_2019_expo.json --verbosity=debug 
cleanup()

echo "===== Azure2019 Expo - IAT Generation ====="
go run cmd/loader.go --config cmd/config_fyp_test_1_2019_expo.json --verbosity=debug --iatGeneration=true
mkdir -p data/out/test_1_2019_expo/iat
mv *.json data/out/test_1_2019_expo/iat

# Azure2019 Shift
echo "===== Azure2019 Shift ====="
go run cmd/loader.go --config cmd/config_fyp_test_1_2019_shift.json --verbosity=debug
cleanup()

echo "===== Azure2019 Shift - IAT Generation ====="
go run cmd/loader.go --config cmd/config_fyp_test_1_2019_shift.json --verbosity=debug --iatGeneration=true
mkdir -p data/out/test_1_2019_shift/iat
mv *.json data/out/test_1_2019_shift/iat

# Azure2021
echo "===== Azure2021 ====="
go run cmd/loader.go --config cmd/config_fyp_test_1_2021.json --verbosity=debug
cleanup()

echo "===== Azure2021 - IAT Generation ====="
go run cmd/loader.go --config cmd/config_fyp_test_1_2021.json --verbosity=debug --iatGeneration=true
mkdir -p data/out/test_1_2021/iat
mv *.json data/out/test_1_2021/iat

echo "END"



