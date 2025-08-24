@echo off
cd /d C:\Users\Administrator\p2p-mesh\relay && go mod download && go build -o ..\bin\relay.exe .
cd /d C:\Users\Administrator\p2p-mesh\node  && go mod download && go build -o ..\bin\node.exe  .
echo Built to .\bin\relay.exe and .\bin\node.exe
