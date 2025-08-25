package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	routingdisc "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/host/autonat"
	pstoremem "github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoremem"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	clientv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	quic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	tcp "github.com/libp2p/go-libp2p/p2p/transport/tcp"

	dht "github.com/libp2p/go-libp2p-kad-dht"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/joho/godotenv"
)

const keyFile = "/data/peerkey.bin"

func loadOrCreateKey() (crypto.PrivKey, error) {
	_ = os.MkdirAll(filepath.Dir(keyFile), 0o755)
	if b, err := os.ReadFile(keyFile); err == nil && len(b) == ed25519.PrivateKeySize {
		return crypto.UnmarshalEd25519PrivateKey(b)
	}
	_, pk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyFile, []byte(pk), 0o600); err != nil {
		return nil, err
	}
	return crypto.UnmarshalEd25519PrivateKey([]byte(pk))
}

type mdnsNotifee struct{ h host.Host }

// HandlePeerFound attempts to connect to peers discovered via mDNS.
func (n *mdnsNotifee) HandlePeerFound(pi peer.AddrInfo) {
	fmt.Printf("[mDNS] found %s\n", short(pi.ID))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = n.h.Connect(ctx, pi)
}

func getenvBool(k string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(k)))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes" || v == "y"
}

func main() {
	_ = godotenv.Load(".env")
	cfg := loadConfig()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// env/config
	room := firstNonEmpty(os.Getenv("APP_ROOM"), cfg.AppRoom, "my-room")
	listenTCP := firstNonEmpty(os.Getenv("LISTEN_TCP"), "/ip4/0.0.0.0/tcp/4001")
	listenQUIC := os.Getenv("LISTEN_QUIC") // e.g. "/ip4/0.0.0.0/udp/4001/quic-v1"
	relayAddr := firstNonEmpty(os.Getenv("RELAY_ADDR"), cfg.RelayAddr)
	enableRelayClient := getenvBool("ENABLE_RELAY_CLIENT", cfg.EnableRelayClient)
	enableHP := getenvBool("ENABLE_HOLEPUNCH", cfg.EnableHolePunch)
	enableUPnP := getenvBool("ENABLE_UPNP", cfg.EnableUPnP)
	peerDB := newPeerStore("/data/known_peers.txt")
	bootstrapPeers := cfg.BootstrapPeers
	envProvided := false
	if envPeers := os.Getenv("BOOTSTRAP_PEERS"); envPeers != "" {
		bootstrapPeers = strings.Split(envPeers, ",")
		envProvided = true
	} else if len(bootstrapPeers) > 0 {
		envProvided = true
	}
	if len(bootstrapPeers) == 0 {
		bootstrapPeers = peerDB.List()
	}
	announceAddrs := []ma.Multiaddr{}
	seeds := cfg.AnnounceAddrs
	if env := os.Getenv("ANNOUNCE_ADDRS"); env != "" {
		seeds = strings.Split(env, ",")
	}
	for _, s := range seeds {
		m, err := ma.NewMultiaddr(strings.TrimSpace(s))
		if err != nil {
			if s != "" {
				fmt.Println("Invalid announce addr, skipping:", err)
			}
			continue
		}
		announceAddrs = append(announceAddrs, m)
	}

	// key & in-memory peerstore
	priv, err := loadOrCreateKey()
	must(err)
	ps, err := pstoremem.NewPeerstore()
	must(err)
	defer ps.Close()

	// resource manager (safe defaults)
	rmgr, err := rcmgr.NewResourceManager(rcmgr.NewFixedLimiter(rcmgr.DefaultLimits.AutoScale()))
	must(err)

	// host options
	opts := []libp2p.Option{
		libp2p.Identity(priv),
		libp2p.Peerstore(ps),
		libp2p.ResourceManager(rmgr),
		libp2p.Muxer(yamux.ID, yamux.DefaultTransport),
		libp2p.Transport(tcp.NewTCPTransport),
	}
	if listenTCP != "" {
		opts = append(opts, libp2p.ListenAddrStrings(listenTCP))
	}
	if listenQUIC != "" {
		opts = append(opts, libp2p.Transport(quic.NewTransport), libp2p.ListenAddrStrings(listenQUIC))
	}
	if enableUPnP {
		opts = append(opts, libp2p.NATPortMap())
	}
	if enableRelayClient {
		opts = append(opts, libp2p.EnableRelay()) // client relay
	}
	if enableHP {
		opts = append(opts, libp2p.EnableHolePunching())
	}
	if len(announceAddrs) > 0 {
		opts = append(opts, libp2p.AddrsFactory(func(addrs []ma.Multiaddr) []ma.Multiaddr {
			return append(addrs, announceAddrs...)
		}))
	}

	h, err := libp2p.New(opts...)
	must(err)
	defer h.Close()

	h.Network().Notify(&network.NotifyBundle{ConnectedF: func(net network.Network, conn network.Conn) {
		peerDB.Add(conn.RemoteMultiaddr(), conn.RemotePeer())
	}})

	fmt.Printf("PeerID: %s\n", h.ID().String())
	for _, a := range h.Addrs() {
		fmt.Printf("Listen: %s/p2p/%s\n", a, h.ID())
	}

	// AutoNAT (help NAT type detection)
	_, _ = autonat.New(h)

	// mDNS for LAN
	ser := mdns.NewMdnsService(h, room, &mdnsNotifee{h: h})
	defer ser.Close()

	// If relay provided, connect to it. Multiple addresses can be supplied
	// comma-separated; we try each until one succeeds.
	if relayAddr != "" {
		connected := false
		for _, addr := range strings.Split(relayAddr, ",") {
			addr = strings.TrimSpace(addr)
			if addr == "" {
				continue
			}
			maddr, err := ma.NewMultiaddr(addr)
			if err != nil {
				fmt.Println("Invalid RELAY_ADDR, skipping:", err)
				continue
			}
			if err := connectToRelay(ctx, h, maddr); err != nil {
				fmt.Println("Relay connect failed:", err)
				continue
			}
			fmt.Println("Relay connected via", addr)
			connected = true
			break
		}
		if !connected {
			fmt.Println("Relay connection attempts exhausted")
		}
	}

	// connect to any configured bootstrap peers
	bootstrapped := false
	for _, addr := range bootstrapPeers {
		a := strings.TrimSpace(addr)
		if a == "" {
			continue
		}
		maddr, err := ma.NewMultiaddr(a)
		if err != nil {
			fmt.Println("Invalid bootstrap addr, skipping:", err)
			continue
		}
		pi, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			fmt.Println("bootstrap AddrInfo error:", err)
			continue
		}
		if pi.ID == h.ID() {
			continue // skip connecting to ourselves
		}
		h.Peerstore().AddAddrs(pi.ID, pi.Addrs, peerstore.PermanentAddrTTL)
		if err := h.Connect(ctx, *pi); err != nil {
			fmt.Println("bootstrap connect failed:", err)
		} else {
			bootstrapped = true
			fmt.Printf("Bootstrapped to %s\n", short(pi.ID))
		}
	}
	if !bootstrapped && envProvided {
		for _, addr := range peerDB.List() {
			a := strings.TrimSpace(addr)
			if a == "" {
				continue
			}
			maddr, err := ma.NewMultiaddr(a)
			if err != nil {
				fmt.Println("Invalid fallback addr, skipping:", err)
				continue
			}
			pi, err := peer.AddrInfoFromP2pAddr(maddr)
			if err != nil {
				fmt.Println("fallback AddrInfo error:", err)
				continue
			}
			if pi.ID == h.ID() {
				continue // skip ourselves from peer DB
			}
			h.Peerstore().AddAddrs(pi.ID, pi.Addrs, peerstore.PermanentAddrTTL)
			if err := h.Connect(ctx, *pi); err != nil {
				fmt.Println("fallback connect failed:", err)
			} else {
				bootstrapped = true
				fmt.Printf("Bootstrapped to %s\n", short(pi.ID))
			}
		}
	}

	// DHT for global peer discovery
	kdht, err := dht.New(ctx, h)
	must(err)
	must(kdht.Bootstrap(ctx))
	rdisc := routingdisc.NewRoutingDiscovery(kdht)
	// advertise our presence and continuously look for peers in the room
	// so newly joined peers are discovered automatically
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			if kdht.RoutingTable().Size() > 0 {
				if _, err := rdisc.Advertise(ctx, "room:"+room); err != nil {
					fmt.Println("DHT advertise error:", err)
				}
				peerCh, err := rdisc.FindPeers(ctx, "room:"+room)
				if err != nil {
					fmt.Println("DHT find peers:", err)
				} else {
					for p := range peerCh {
						if p.ID == h.ID() {
							continue
						}
						fmt.Printf("[DHT] found %s\n", short(p.ID))
						_ = h.Connect(ctx, p)
					}
				}
			}
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()

	// PubSub topic
	psub, err := pubsub.NewGossipSub(ctx, h)
	must(err)
	topic, err := psub.Join("room:" + room)
	must(err)
	sub, err := topic.Subscribe()
	must(err)

	RunWebGateway(ctx, h, psub, topic, sub, room)

	// simple handler: print any direct stream
	h.SetStreamHandler("/echo/1.0.0", func(s network.Stream) {
		defer s.Close()
		io.Copy(os.Stdout, s)
	})

	// publisher: read stdin and publish to the topic
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			if err := topic.Publish(ctx, []byte(line)); err != nil {
				fmt.Println("publish error:", err)
			}
		}
	}()

	// wait signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}

func connectToRelay(ctx context.Context, h host.Host, maddr ma.Multiaddr) error {
	pi, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		return err
	}
	h.Peerstore().AddAddrs(pi.ID, pi.Addrs, peerstore.PermanentAddrTTL)
	if err := h.Connect(ctx, *pi); err != nil {
		return err
	}
	// Reserve slot (optional; ensures we can use relay/circuit)
	_, err = clientv2.Reserve(ctx, h, *pi)
	return err
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
func short(id peer.ID) string {
	b := []byte(id)
	if len(b) > 6 {
		return hex.EncodeToString(b[:6])
	}
	return id.String()
}
