package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

type peerStore struct {
	path string
	mu   sync.Mutex
	set  map[string]struct{}
}

func newPeerStore(path string) *peerStore {
	ps := &peerStore{path: path, set: map[string]struct{}{}}
	if f, err := os.Open(path); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			s := strings.TrimSpace(scanner.Text())
			if s != "" {
				ps.set[s] = struct{}{}
			}
		}
	}
	return ps
}

func (ps *peerStore) List() []string {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	out := make([]string, 0, len(ps.set))
	for s := range ps.set {
		out = append(out, s)
	}
	return out
}

func (ps *peerStore) Add(addr ma.Multiaddr, id peer.ID) {
	m := addr.Encapsulate(ma.StringCast("/p2p/" + id.String()))
	s := m.String()
	ps.mu.Lock()
	if _, ok := ps.set[s]; ok {
		ps.mu.Unlock()
		return
	}
	ps.set[s] = struct{}{}
	ps.mu.Unlock()
	os.MkdirAll(filepath.Dir(ps.path), 0o755)
	f, err := os.OpenFile(ps.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(s + "\n")
}
