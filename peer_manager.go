package main

import (
	"context"
	"log"

	"github.com/libp2p/go-libp2p-core/peer"
)

type PeerManager struct {
	config Config
	peers  map[peer.ID]ID // TODO locking
}

func NewPeerManager(ctx context.Context, config Config) *PeerManager {
	return &PeerManager{
		config: config,
		peers:  map[peer.ID]ID{},
	}
}

func (m *PeerManager) IsKnownPeer(peerID peer.ID) bool {
	_, found := m.peers[peerID]
	return found
}

func (m *PeerManager) AddPeer(p ID) {
	if _, found := m.peers[p.ID]; !found {
		m.peers[p.ID] = p
		log.Printf("Added peer %s from cluster %s", p.ID, p.ClusterID)
	}
}

func (m *PeerManager) ClusterPeers() IDs {
	var peers IDs
	for _, p := range m.peers {
		if p.ClusterID == m.config.ClusterID {
			peers = append(peers, p)
		}
	}
	return peers
}

func (m *PeerManager) AllIDs() IDs {
	var peers IDs
	for _, p := range m.peers {
		peers = append(peers, p)
	}
	return peers
}

func (m *PeerManager) PeersPerCluster() map[string]IDs {
	var peers = map[string]IDs{}

	for _, p := range m.peers {
		if p.ClusterID != m.config.ClusterID {
			peers[p.ClusterID] = append(peers[p.ClusterID], p)
		}
	}

	return peers
}
