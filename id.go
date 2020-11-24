package main

import (
	"github.com/libp2p/go-libp2p-core/peer"
)

type ID struct {
	ID        peer.ID   `json:"id"`
	ClusterID string    `json:"cluster_id"`
	Addresses []string  `json:"addresses"`
	Peers     []peer.ID `json:"peers"`
}

type IDs []ID

func (ids IDs) PeerIDs() peer.IDSlice {
	var peerIDs peer.IDSlice
	for _, id := range ids {
		peerIDs = append(peerIDs, id.ID)
	}
	return peerIDs
}
