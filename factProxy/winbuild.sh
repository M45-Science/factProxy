#!/bin/bash
GOOS=windows GOARCH=amd64 go build -o M45-Factorio-Proxy.exe
zip winProxy.zip M45-Factorio-Proxy.exe readme.txt connect-links.html
