#!/usr/bin/env bash

# The image will be pushed in the repository specified in the 1st argument with tag specified in the 3rd argument.
# The 2nd argument defines the size of the dummy file in MiB.

REPOSITORY="$1" # docker repository
MBS="$2" # size of the added dummy file in MiB
FUNC_NAME="$3" # tag name
FILE_SIZE="$MBS""M"

docker build --build-arg FUNC_TYPE=TRACE \
	--build-arg FUNC_PORT=80 \
	-f Dockerfile.trace \
	-t $REPOSITORY .
docker create --name $FUNC_NAME $REPOSITORY
dd if=/dev/urandom of=$FILE_SIZE bs=$FILE_SIZE count=1
docker cp $FILE_SIZE $FUNC_NAME:$FILE_SIZE
docker commit $FUNC_NAME $REPOSITORY:$FUNC_NAME
docker push $REPOSITORY:$FUNC_NAME
rm $FILE_SIZE

