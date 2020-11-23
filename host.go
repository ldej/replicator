package main

import (
	"context"
	"crypto/rand"
	"log"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/multiformats/go-multiaddr"
)

func NewHost(ctx context.Context, config Config) host.Host {
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, rand.Reader)
	if err != nil {
		log.Fatal(err)
	}

	h, err := libp2p.New(ctx,
		libp2p.ListenAddrs([]multiaddr.Multiaddr(config.ListenAddresses)...),
		libp2p.Identity(priv),
	)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Host ID: %s", h.ID().Pretty())
	log.Printf("Connect to me on:")
	for _, addr := range h.Addrs() {
		log.Printf("  - %s/p2p/%s", addr, h.ID().Pretty())
	}
	return h
}
