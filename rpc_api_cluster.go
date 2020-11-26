package main

import (
	"context"

	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	ClusterServiceName        = "ClusterRPCAPI"
	ClusterIDFuncName         = "ID"
	ClusterRemovePeerFuncName = "RemovePeer"
)

type ClusterRPCAPI struct {
	c *Cluster
}

func (c *ClusterRPCAPI) ID(ctx context.Context, in struct{}, out *ID) error {
	id := c.c.ID(ctx)
	*out = *id
	return nil
}

func (c *ClusterRPCAPI) RemovePeer(ctx context.Context, in peer.ID, out *struct{}) error {
	return c.c.RemovePeer(in)
}
