#!/bin/bash
echo "START"

echo "===== Azure2019 Run ====="
go run cmd/loader.go --config cmd/config_fyp_azure2019.json --verbosity=debug

echo "===== Azure2021 Run ====="
go run cmd/loader.go --config cmd/config_fyp_azure2021.json --verbosity=debug

echo "===== Huawei2023 Run ====="
go run cmd/loader.go --config cmd/config_fyp_huawei2023.json --verbosity=debug

echo "===== IBM2026 Run ====="
go run cmd/loader.go --config cmd/config_fyp_ibm2026.json --verbosity=debug

echo "END"
