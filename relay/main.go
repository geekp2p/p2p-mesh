package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	libp2p "github.com/libp2p/go-libp2p"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"

	"github.com/joho/godotenv"
)

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

	// สร้าง host ที่ฟังที่ listen address
	h, err := libp2p.New(libp2p.ListenAddrStrings(listen), libp2p.EnableRelay())
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
