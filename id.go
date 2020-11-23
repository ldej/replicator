package main

import (
	"github.com/libp2p/go-libp2p-core/peer"
)

type ID struct {
	ID        peer.ID   `json:"id"`
	Addresses []string  `json:"addresses"`
	Peers     []peer.ID `json:"peers"`
}
