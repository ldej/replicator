package main

import (
	"context"
	"log"
	"time"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/libp2p/go-libp2p-discovery"
	"github.com/libp2p/go-libp2p-gorpc"
)

type Cluster struct {
	ctx  context.Context
	id   peer.ID
	host host.Host

	config Config

	rpcServer        *rpc.Server
	rpcClient        *rpc.Client
	routingDiscovery *discovery.RoutingDiscovery

	peerManager *PeerManager

	replicationService *ReplicationService
}

func NewCluster(
	ctx context.Context,
	host host.Host,
	config Config,
	replicationService *ReplicationService,
	peerManager *PeerManager,
) (*Cluster, error) {

	kdht, err := NewKDHT(ctx, host, config)
	if err != nil {
		return nil, err
	}

	c := &Cluster{
		ctx:                ctx,
		id:                 host.ID(),
		host:               host,
		config:             config,
		rpcServer:          nil,
		rpcClient:          nil,
		routingDiscovery:   kdht,
		replicationService: replicationService,
		peerManager:        peerManager,
	}

	// Add ourselves
	id := c.ID(ctx)
	c.peerManager.AddPeer(*id)

	// Start discovering peers
	go c.discover()

	err = c.setupRpc()
	if err != nil {
		return nil, err
	}

	c.setupRPCClients()

	return c, nil
}

func (c *Cluster) setupRpc() error {
	rpcServer := rpc.NewServer(c.host, protocol.ID(c.config.ProtocolID))

	replicationRpc := ReplicationRPCAPI{r: c.replicationService}
	err := rpcServer.Register(&replicationRpc)
	if err != nil {
		return err
	}

	clusterRpc := ClusterRPCAPI{c: c}
	err = rpcServer.Register(&clusterRpc)
	if err != nil {
		return err
	}

	c.rpcServer = rpcServer
	c.rpcClient = rpc.NewClientWithServer(c.host, protocol.ID(c.config.ProtocolID), rpcServer)

	return nil
}

func (c *Cluster) setupRPCClients() {
	c.replicationService.rpcClient = c.rpcClient
}

func (c *Cluster) discover() {
	for {
		discovery.Advertise(c.ctx, c.routingDiscovery, c.config.Rendezvous)
		// log.Println("Successfully announced!")

		// log.Println("Searching for other peers...")
		peerChan, err := c.routingDiscovery.FindPeers(c.ctx, c.config.Rendezvous)
		if err != nil {
			log.Fatal(err)
		}

		for p := range peerChan {
			if p.ID == c.id {
				continue
			}

			if c.host.Network().Connectedness(p.ID) != network.Connected {
				_, err := c.host.Network().DialPeer(c.ctx, p.ID)
				if err != nil {
					// log.Printf("Failed to connect to %s: %-v", p.ID, err)
					continue
				}
			}

			if !c.peerManager.IsKnownPeer(p.ID) {
				var id ID
				err = c.rpcClient.Call(
					p.ID,
					ClusterServiceName,
					ClusterIDFuncName,
					struct{}{},
					&id,
				)
				if err != nil {
					log.Printf("Failed to get peer ID: %-v", err)
					continue
				}
				c.peerManager.AddPeer(id)
			}
		}
		time.Sleep(time.Second * 1)
	}
}

func (c *Cluster) knownPeer(peer peer.ID) bool {
	for _, p := range c.host.Peerstore().Peers() {
		if peer == p {
			return true
		}
	}
	return false
}

func (c *Cluster) Peers(ctx context.Context) ([]*ID, error) {
	peers := c.host.Peerstore().Peers()
	lenPeers := len(peers)

	ctxts := Ctxts(lenPeers)

	ids := make([]*ID, lenPeers)

	errs := c.rpcClient.MultiCall(
		ctxts,
		peers,
		ClusterServiceName,
		ClusterIDFuncName,
		struct{}{},
		CopyIDsToIfaces(ids),
	)
	for _, err := range errs {
		if err != nil {
			log.Printf("Err %-v", err)
		}
	}
	return ids, nil
}

func (c *Cluster) ID(ctx context.Context) *ID {
	// msgpack decode error [pos 108]: interface conversion: *multiaddr.Multiaddr is not
	// encoding.BinaryUnmarshaler: missing method UnmarshalBinary
	var addrs []string
	for _, addr := range c.host.Addrs() {
		addrs = append(addrs, addr.String())
	}

	return &ID{
		ID:        c.id,
		ClusterID: c.config.ClusterID,
		Addresses: addrs,
		Peers:     c.host.Peerstore().Peers(),
	}
}

func Ctxts(n int) []context.Context {
	ctxs := make([]context.Context, n)
	for i := 0; i < n; i++ {
		ctxs[i] = context.Background()
	}
	return ctxs
}

func CopyIDsToIfaces(in []*ID) []interface{} {
	ifaces := make([]interface{}, len(in))
	for i := range in {
		in[i] = &ID{}
		ifaces[i] = in[i]
	}
	return ifaces
}
