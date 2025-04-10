#!/bin/bash
GOOS=windows GOARCH=amd64 go build -o M45-Relay-Client.exe
zip M45-Relay.zip M45-Relay-Client.exe readme.txt connect-links.html
