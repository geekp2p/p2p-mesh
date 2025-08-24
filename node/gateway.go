package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	host "github.com/libp2p/go-libp2p/core/host"
)

type ChatMsg struct {
	From string `json:"from"`
	Text string `json:"text"`
	Ts   int64  `json:"ts"`
}

type WSClient struct {
	conn *websocket.Conn
	send chan []byte
}

type Gateway struct {
	h        host.Host
	ps       *pubsub.PubSub
	topic    *pubsub.Topic
	sub      *pubsub.Subscription
	clients  map[*WSClient]bool
	mu       sync.RWMutex
	upgrader websocket.Upgrader
	nick     string
	room     string
}

func NewGateway(h host.Host, ps *pubsub.PubSub, room, nick string) *Gateway {
	return &Gateway{
		h:       h,
		ps:      ps,
		clients: make(map[*WSClient]bool),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
		nick: nick,
		room: room,
	}
}

func (g *Gateway) Start(ctx context.Context, webAddr string) error {
	var err error
	g.topic, err = g.ps.Join(g.room)
	if err != nil {
		return err
	}
	g.sub, err = g.topic.Subscribe()
	if err != nil {
		return err
	}

	// consume pubsub -> fanout to websockets
	go func() {
		for {
			msg, err := g.sub.Next(ctx)
			if err != nil {
				log.Println("pubsub sub.Next:", err)
				return
			}
			var cm ChatMsg
			if err := json.Unmarshal(msg.Data, &cm); err != nil {
				continue
			}
			g.broadcast(cm)
		}
	}()

	http.HandleFunc("/", g.serveIndex)
	http.HandleFunc("/ws", g.serveWS)

	log.Printf("🌐 Chat UI on http://0.0.0.0%s  (room=%s nick=%s)\n", webAddr, g.room, g.nick)
	return http.ListenAndServe(webAddr, nil)
}

func (g *Gateway) serveIndex(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "web/index.html")
}

func (g *Gateway) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := g.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client := &WSClient{conn: conn, send: make(chan []byte, 32)}
	g.mu.Lock()
	g.clients[client] = true
	g.mu.Unlock()

	// writer
	go func() {
		for b := range client.send {
			_ = client.conn.WriteMessage(websocket.TextMessage, b)
		}
	}()

	// reader -> publish to GossipSub
	go func() {
		defer func() {
			g.mu.Lock()
			delete(g.clients, client)
			g.mu.Unlock()
			_ = conn.Close()
		}()
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			cm := ChatMsg{
				From: g.nick,
				Text: string(data),
				Ts:   time.Now().Unix(),
			}
			payload, _ := json.Marshal(cm)
			if err := g.topic.Publish(context.Background(), payload); err != nil {
				log.Println("topic.Publish:", err)
			}
		}
	}()
}

func (g *Gateway) broadcast(cm ChatMsg) {
	b, _ := json.Marshal(cm)
	g.mu.RLock()
	defer g.mu.RUnlock()
	for c := range g.clients {
		select {
		case c.send <- b:
		default:
		}
	}
}

func RunWebGateway(ctx context.Context, h host.Host, ps *pubsub.PubSub, room string) {
	webAddr := os.Getenv("WEB_ADDR")
	if webAddr == "" {
		webAddr = ":8080"
	}
	nick := os.Getenv("NODE_NICK")
	if nick == "" {
		nick = h.ID().String()[2:8]
	}
	gw := NewGateway(h, ps, room, nick)
	go func() {
		if err := gw.Start(ctx, webAddr); err != nil {
			log.Println("gateway.Start:", err)
		}
	}()
}
