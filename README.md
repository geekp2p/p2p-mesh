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
cp .env.example .env   # edit APP_ROOM to pick a room name
docker compose --env-file .env up --build
```

All containers read configuration from the `.env` file. Peers that use the
same `APP_ROOM` value will automatically discover and join each other.

## 🔌 Enabling Relay Client

Enable the relay client if your nodes must dial through a public relay server. Set
`ENABLE_RELAY_CLIENT` and provide the relay's multiaddress in the `.env` file
before starting the containers:
␊
```bash␊
echo "ENABLE_RELAY_CLIENT=true" >> .env
echo "RELAY_ADDR=/ip4/<RELAY_IP>/tcp/4003/p2p/<RELAY_PEER_ID>" >> .env
docker compose --env-file .env up --build
```␊

## 💬 Chat

Open the chat web UI for each node in your browser:

- http://localhost:3001 for `node1`
- http://localhost:3002 for `node2`

Enter a nickname when prompted and start chatting. Messages will be broadcast to all peers connected to the mesh.

## 🌍 Bootstrapping & DHT

Nodes can discover each other globally using a Kademlia DHT. Provide one or more
public bootstrap peers via the `BOOTSTRAP_PEERS` variable in `.env` or the
`bootstrap_peers` entry in `config.yaml`:

```bash
echo "BOOTSTRAP_PEERS=/ip4/<NODE_IP>/tcp/4001/p2p/<NODE_PEER_ID>,/dns4/example.com/tcp/4001/p2p/<NODE_PEER_ID>" >> .env
```

Each address must be a full multiaddress **for another node** (not a relay)
including its peer ID. Nodes will connect to the bootstrap peers and announce
themselves on the DHT so that others can find and communicate with them. If you
see `DHT advertise error: failed to find any peer in table`, ensure that at
least one bootstrap peer is reachable and running the DHT.

## 🌐 Announcing Public Addresses

Containers typically advertise their internal addresses (e.g. `127.0.0.1` or
`172.x.x.x`). If you want other machines to dial your relay or nodes directly,
override the announced addresses with the `ANNOUNCE_ADDRS` environment variable.
The Compose file exposes helper variables so you can set them in `.env`:

```bash
RELAY_ANNOUNCE=/ip4/<PUBLIC_IP>/tcp/4003
NODE1_ANNOUNCE=/ip4/<PUBLIC_IP>/tcp/4001
NODE2_ANNOUNCE=/ip4/<PUBLIC_IP>/tcp/4002
```

Multiple addresses can be provided comma-separated. Peers will advertise these
public addresses in addition to their default ones, improving reachability when
running behind NAT or in Docker.

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