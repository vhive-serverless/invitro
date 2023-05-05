#!/usr/bin/env bash

if [[ $# -ne 2 ]]
then
    echo "Proper calling format:"
    echo "$ bash ./precompile_go_openwhisk.sh    <path to .go input file>    <path to .zip output file>"
    exit 1
fi

sudo docker run openwhisk/action-golang-v1.17 -compile main <$1 >$2