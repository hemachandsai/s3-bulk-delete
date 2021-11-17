@echo off
echo "Started Build Process"
set GOOS=darwin
go build -o s3-bulk-delete-mac
echo "Done building for Mac"
set GOOS=linux
go build -o s3-bulk-delete-linux
echo "Done building for Linux"
set GOOS=windows
go build -o s3-bulk-delete-windows.exe
echo "Done building for Windows"

if not exist ".\binaries" mkdir .\binaries
move .\s3-bulk-delete* .\binaries