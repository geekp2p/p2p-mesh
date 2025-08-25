# p2p-mesh

A peer-to-peer mesh networking project written in Go and containerized with Docker.
Each node runs independently with a persistent unique PeerID and can discover peers automatically on LAN or over the internet.
The system is serverless by design, with optional relay support for guaranteed connectivity behind restrictive NATs.

## ✨ Features
- ✅ Unique peer IDs for nodes and relays (Ed25519, persisted in volume)
- ✅ LAN discovery using mDNS
- ✅ NAT traversal with AutoNAT, UPnP, NAT-PMP, and hole punching (DCUtR)
- ✅ GossipSub pub/sub messaging between peers
- ✅ Optional Circuit Relay v2 for guaranteed connectivity
- ✅ Global peer discovery using Kademlia DHT with optional bootstrap peers
- ✅ Docker Compose setup for easy multi-node deployment
## 📦 Quick Start

Clone and run two local nodes in Docker:

```bash
git clone https://github.com/geekp2p/p2p-mesh.git
cd p2p-mesh
docker compose up --build
```

## 🔌 Enabling Relay Client

Enable the relay client if your nodes must dial through a public relay server. Set the
`ENABLE_RELAY_CLIENT` flag and provide the relay's multiaddress before starting the
containers:

```bash
export ENABLE_RELAY_CLIENT=true
export RELAY_ADDR=/ip4/<RELAY_IP>/tcp/4003/p2p/<RELAY_PEER_ID>
docker compose up --build
```

## 💬 Chat

Open the chat web UI for each node in your browser:

- http://localhost:3001 for `node1`
- http://localhost:3002 for `node2`

Enter a nickname when prompted and start chatting. Messages will be broadcast to all peers connected to the mesh.

## 🌍 Bootstrapping & DHT

Nodes can discover each other globally using a Kademlia DHT. Provide one or more
public bootstrap peers via the `BOOTSTRAP_PEERS` environment variable or the
`bootstrap_peers` entry in `config.yaml`:

```bash
export BOOTSTRAP_PEERS=/ip4/<IP>/tcp/<PORT>/p2p/<PEER_ID>,/dns4/example.com/tcp/4001/p2p/<PEER_ID>
```

Each address should be a full multiaddress including the peer ID. New nodes will
connect to the bootstrap peers and announce themselves on the DHT so that others
can find and communicate with them.

## 🛠 Manual Build

### Node

```bash
cd node
go build -o p2p-node .
./p2p-node
```

### Relay

```bash
cd relay
go build -o p2p-relay .
./p2p-relay
```