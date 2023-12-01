#!/usr/bin/env bash

REPOSITORY="$1" # docker repository
MBS="$2" # size of the added dummy file in MB
TRACE_NUM="trace-$MBS"
FILE_SIZE="$MBS""M"

docker build --build-arg FUNC_TYPE=TRACE \
	--build-arg FUNC_PORT=80 \
	-f Dockerfile.trace \
	-t $REPOSITORY .
docker create --name $TRACE_NUM $REPOSITORY
dd if=/dev/urandom of=$FILE_SIZE bs=$FILE_SIZE count=1
docker cp $FILE_SIZE $TRACE_NUM:$FILE_SIZE
docker commit $TRACE_NUM $REPOSITORY
docker push $REPOSITORY
rm $FILE_SIZE

