package main

import (
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
	"github.com/libp2p/go-libp2p/p2p/host/autonat"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoreds"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	quic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	tcp "github.com/libp2p/go-libp2p/p2p/transport/tcp"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	ma "github.com/multiformats/go-multiaddr"
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

func (n *mdnsNotifee) HandlePeerFound(pi peer.AddrInfo) {
	_ = n.h.Connect(context.Background(), pi)
}

func getenvBool(k string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(k)))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes" || v == "y"
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// env
	room := os.Getenv("APP_ROOM")
	if room == "" {
		room = "my-room"
	}
	listenTCP := firstNonEmpty(os.Getenv("LISTEN_TCP"), "/ip4/0.0.0.0/tcp/4001")
	listenQUIC := os.Getenv("LISTEN_QUIC") // e.g. "/ip4/0.0.0.0/udp/4001/quic-v1"
	relayAddr := os.Getenv("RELAY_ADDR")
	enableRelayClient := getenvBool("ENABLE_RELAY_CLIENT", true)
	enableHP := getenvBool("ENABLE_HOLEPUNCH", true)
	enableUPnP := getenvBool("ENABLE_UPNP", true)

	// key & peerstore
	priv, err := loadOrCreateKey()
	must(err)
	ps, err := pstoreds.NewPeerstore(ctx, "", nil)
	must(err)
	defer ps.Close()

	// resource manager (safe defaults)
	rmgr, err := rcmgr.NewResourceManager(rcmgr.NewFixedLimiter(rcmgr.DefaultLimits.AutoScale()))
	must(err)

	// host options
	opts := []libp2p.Option{
		libp2p.Identity(priv),
		libp2p.ResourceManager(rmgr),
		libp2p.Muxer(yamux.DefaultID, yamux.DefaultTransport),
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

	h, err := libp2p.New(opts...)
	must(err)
	defer h.Close()

	fmt.Printf("PeerID: %s\n", h.ID().String())
	for _, a := range h.Addrs() {
		fmt.Printf("Listen: %s/p2p/%s\n", a, h.ID())
	}

	// AutoNAT (help NAT type detection)
	_, _ = autonat.New(ctx, h, autonat.EnableService(false))

	// mDNS for LAN
	ser, err := mdns.NewMdnsService(h, room, &mdnsNotifee{h: h})
	must(err)
	defer ser.Close()

	// If relay provided, connect to it; that gives us a control path for DCUtR
	if relayAddr != "" {
		if err := connectToRelay(ctx, h, relayAddr); err != nil {
			fmt.Println("Relay connect failed:", err)
		} else {
			fmt.Println("Relay connected.")
		}
	}

	// PubSub topic
	psub, err := pubsub.NewGossipSub(ctx, h)
	must(err)
	topic, err := psub.Join("room:" + room)
	must(err)
	sub, err := topic.Subscribe()
	must(err)

	// simple handler: print any direct stream
	h.SetStreamHandler("/echo/1.0.0", func(s network.Stream) {
		defer s.Close()
		io.Copy(os.Stdout, s)
	})

	// publisher
	go func() {
		tkr := time.NewTicker(5 * time.Second)
		defer tkr.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tkr.C:
				msg := fmt.Sprintf("hello from %s @ %s", short(h.ID()), time.Now().Format(time.RFC3339))
				_ = topic.Publish(ctx, []byte(msg))
			}
		}
	}()

	// subscriber
	go func() {
		for {
			msg, err := sub.Next(ctx)
			if err != nil {
				return
			}
			if msg.ReceivedFrom != h.ID() {
				fmt.Printf("[pubsub] %s: %s\n", short(msg.ReceivedFrom), string(msg.Data))
			}
		}
	}()

	// wait signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}

func connectToRelay(ctx context.Context, h host.Host, relay string) error {
	maddr, err := ma.NewMultiaddr(relay)
	if err != nil {
		return err
	}
	pi, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		return err
	}
	h.Peerstore().AddAddrs(pi.ID, pi.Addrs, peerstore.PermanentAddrTTL)
	if err := h.Connect(ctx, *pi); err != nil {
		return err
	}
	// Reserve slot (optional; ensures we can use relay/circuit)
	_, err = relayv2.Reserve(ctx, h, *pi)
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
