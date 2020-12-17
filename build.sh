#!/bin/bash
echo "Started Build Process"
env GOOS=windows go build -o s3-bulk-delete-windows.exe
echo "Done building for Windows"
env GOOS=darwin go build -o s3-bulk-delete-mac
echo "Done building for Mac"
env GOOS=linux go build -o s3-bulk-delete-linux
echo "Done building for Linux"
DIR="/binaries"
if [ -d "$DIR" ]; then
    mkdir bin
fi
mv s3-bulk-delete* binaries/
