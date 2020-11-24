package main

import (
	"context"
)

const (
	ClusterServiceName = "ClusterRPCAPI"
	ClusterIDFuncName = "ID"
)

type ClusterRPCAPI struct {
	c *Cluster
}

func (c *ClusterRPCAPI) ID(ctx context.Context, in struct{}, out *ID) error {
	id := c.c.ID(ctx)
	*out = *id
	return nil
}