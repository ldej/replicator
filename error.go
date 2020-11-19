package main

import (
	"errors"

	"github.com/libp2p/go-libp2p-core/peer"
)

var (
	ErrClusterFull = errors.New("cluster is full")
	ErrNotLeader = errors.New("peer is not the leader")
	ErrNotInCluster = errors.New("not in cluster")
)

type PeerError struct {
	PeerID string `json:"peerId"`
	ErrorMessage string `json:"error"`
}

type PeerErrors []PeerError

func MapPeerErrors(peers peer.IDSlice, errors []error) PeerErrors {
	var peerErrors PeerErrors
	for i, err := range errors {
		if err != nil {
			peerErrors = append(peerErrors, PeerError{
				PeerID:       peers[i].String(),
				ErrorMessage: err.Error(),
			})
		}
	}
	return peerErrors
}
