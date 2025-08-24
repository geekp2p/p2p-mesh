FROM golang:1.24-alpine AS build
WORKDIR /src
RUN apk add --no-cache upx
COPY . .
RUN cat > main.go <<'EOF'
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	libp2p "github.com/libp2p/go-libp2p"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
)
func main() {
	listen := os.Getenv("RELAY_LISTEN")
	if listen == "" {
		listen = "/ip4/0.0.0.0/tcp/4003"
	}
	h, err := libp2p.New(libp2p.ListenAddrStrings(listen), libp2p.EnableRelay())
	if err != nil { panic(err) }
	defer h.Close()

	// เปิดเป็นรีเลย์ v2
	_, err = relayv2.New(h)
	if err != nil { panic(err) }

	fmt.Printf("Relay PeerID: %s\n", h.ID().String())
	for _, a := range h.Addrs() {
		fmt.Printf("Listen: %s/p2p/%s\n", a, h.ID())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	_ = ctx
}
EOF
RUN CGO_ENABLED=0 go build -o /bin/p2p-relay ./main.go && upx /bin/p2p-relay

FROM alpine:3.20
WORKDIR /app
COPY --from=build /bin/p2p-relay /usr/local/bin/p2p-relay
VOLUME ["/data"]
ENV RELAY_LISTEN=/ip4/0.0.0.0/tcp/4003
ENTRYPOINT ["/usr/local/bin/p2p-relay"]
