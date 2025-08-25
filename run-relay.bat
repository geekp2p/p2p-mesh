@echo off
cd /d C:\Users\Administrator\p2p-mesh
REM Relay does not need a room, but set default to match other nodes
set APP_ROOM=my-room
.\bin\relay.exe