package main

import (
	"context"

	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	ClusterServiceName = "ClusterRPCAPI"
	ClusterAddPeerFuncName = "AddPeer"
	ClusterIDFuncName = "ID"
	ClusterUpdatePeersFuncName = "UpdatePeers"
	ClusterJoinFuncName = "Join"

)

type ClusterRPCAPI struct {
	c *Cluster
}

func (c *ClusterRPCAPI) AddPeer(ctx context.Context, in peer.ID, out *struct{}) error {
	return c.c.AddPeer(ctx, in)
}

func (c *ClusterRPCAPI) UpdatePeers(ctx context.Context, in []peer.ID, out *struct{}) error {
	c.c.clusterPeers = in
	return nil
}

func (c *ClusterRPCAPI) Peers(ctx context.Context, in struct{}, out *peer.IDSlice) error {
	*out = c.c.clusterPeers
	return nil
}

func (c *ClusterRPCAPI) Join(ctx context.Context, in peer.ID, out *struct{}) error {
	return c.c.Join(ctx, in)
}

func (c *ClusterRPCAPI) Bootstrap(ctx context.Context, in struct {}, out *struct{}) error {
	c.c.bootstrapping = true
	return nil
}

func (c *ClusterRPCAPI) ID(ctx context.Context, in struct{}, out *ID) error {
	id := c.c.ID(ctx)
	*out = *id
	return nil
}