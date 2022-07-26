echo "Resolving modules in $(pwd)"
PATHS=$(find . -mindepth 1 -type f -name go.mod -printf '{"workdir":"%h"},')
echo "::set-output name=matrix::{\"include\":[${PATHS%?}]}"
