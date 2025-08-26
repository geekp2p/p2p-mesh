# Project Checklist

## ✅ Completed
- [x] Connect to multiple relays with automatic reconnection
  Ensures a node rotates through provided relay addresses if a relay disconnects.
- [x] Announce and update public nodes in the DHT for automatic bootstrapping
- [x] Implement a watchdog to detect peer disconnects and attempt reconnection
- [x] Broadcast online relay lists via gossipsub so peers learn new relays quickly
- [x] Add a retry schedule to recover the network when all peers have disconnected

## 🚧 In Progress / To Do
- [ ] Design auto-relay fallback so private nodes can reconnect through any available public node
- [ ] Persist known multiaddresses so nodes can rediscover each other after downtime

- [ ] Document the above mechanisms and provide deployment examples