@echo off
cd /d C:\Users\Administrator\p2p-mesh
REM Default chat room must match the Docker/config.yaml setup
set APP_ROOM=my-room
set ENABLE_RELAY_CLIENT=1
set ENABLE_HOLEPUNCH=0
set ENABLE_UPNP=0
.\bin\node.exe