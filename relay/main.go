package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"

	"github.com/joho/godotenv"
)

const keyFile = "/data/relaykey.bin"

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

func main() {
	_ = godotenv.Load(".env")
	cfg := loadConfig()

	// อ่าน config จาก ENV/ไฟล์
	listen := os.Getenv("RELAY_LISTEN")
	if listen == "" {
		listen = cfg.RelayListen
	}
	if listen == "" {
		listen = "/ip4/0.0.0.0/tcp/4003"
	}

	// สร้างหรือโหลดคีย์ส่วนตัวเพื่อให้ PeerID คงที่
	priv, err := loadOrCreateKey()
	if err != nil {
		panic(err)
	}

	// สร้าง host ที่ฟังที่ listen address
	h, err := libp2p.New(
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings(listen),
		libp2p.EnableRelay(),
	)
	if err != nil {
		panic(err)
	}
	defer h.Close()

	// เปิด Circuit Relay v2
	_, err = relayv2.New(h)
	if err != nil {
		panic(err)
	}

	fmt.Printf("✅ Relay PeerID: %s\n", h.ID())
	for _, a := range h.Addrs() {
		fmt.Printf("📡 Listening on: %s/p2p/%s\n", a, h.ID())
	}

	// รอ signal เพื่อปิด
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	fmt.Println("🛑 Shutting down relay...")
	_ = ctx
}
