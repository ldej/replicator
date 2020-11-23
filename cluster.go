package main

import (
	"context"
	"log"
	"sort"
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

	allPeers     peer.IDSlice
	clusterPeers peer.IDSlice

	replicationService *ReplicationService

	bootstrapping bool
}

func NewCluster(
	ctx context.Context,
	host host.Host,
	config Config,
	replicationService *ReplicationService,
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
	}

	c.addPeer(c.id)
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

	consensusRpc := ClusterRPCAPI{c: c}
	err = rpcServer.Register(&consensusRpc)
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

			if c.knownPeer(p.ID) {
				continue
			}

			if c.host.Network().Connectedness(p.ID) != network.Connected {
				if _, err := c.host.Network().DialPeer(c.ctx, p.ID); err != nil {
					//log.Printf("Failed to connect to %s: %-v", p.ID, err)
					continue
				}
			}

			log.Printf(" -> Connected to %s", p.ID)

			c.addPeer(p.ID)
		}
		time.Sleep(time.Second * 15)
	}
}

func (c *Cluster) knownPeer(peer peer.ID) bool {
	for _, p := range c.allPeers {
		if p == peer {
			return true
		}
	}
	return false
}

func (c *Cluster) addPeer(p peer.ID) {
	for _, a := range c.allPeers {
		if a == p {
			return
		}
	}
	c.allPeers = append(c.allPeers, p)
	sort.Sort(c.allPeers)
}

func (c *Cluster) addClusterPeer(p peer.ID) {
	for _, a := range c.clusterPeers {
		if a == p {
			return
		}
	}
	c.clusterPeers = append(c.clusterPeers, p)
	sort.Sort(c.clusterPeers)
	log.Printf("Added cluster peer %s", p)
}

// Add a peer to this Cluster
func (c *Cluster) AddPeer(ctx context.Context, p peer.ID) error {
	if c.bootstrapping {
		c.addClusterPeer(p)
		return nil
	}

	if len(c.clusterPeers) == 0 {
		log.Printf("Peer %s can't join, I'm not in a cluster", p)
		return ErrNotInCluster
	}

	c.addClusterPeer(p)
	return nil
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
	// msgpack decode error [pos 108]: interface conversion: *multiaddr.Multiaddr is not encoding.BinaryUnmarshaler: missing method UnmarshalBinary
	var addrs []string
	for _, addr := range c.host.Addrs() {
		addrs = append(addrs, addr.String())
	}

	return &ID{
		ID: c.id,
		Addresses: addrs,
		Peers: c.host.Peerstore().Peers(),
	}
}

// Join another cluster
func (c *Cluster) Join(ctx context.Context, p peer.ID) error {
	if p == c.id {
		log.Printf("Can't join myself")
		return nil
	}

	log.Printf("I'm going to join cluster %s", p.String())
	err := c.rpcClient.Call(p, ClusterServiceName, ClusterAddPeerFuncName, c.id, &struct{}{})
	if err != nil {
		log.Printf("Cannot join cluster %s: %-v", p.String(), err)
		return err
	}
	return nil
}

func Ctxts(n int) []context.Context {
	ctxs := make([]context.Context, n)
	for i := 0; i < n; i++ {
		ctxs[i] = context.Background()
	}
	return ctxs
}

func RPCDiscardReplies(n int) []interface{} {
	replies := make([]struct{}, n)
	return CopyEmptyStructToIfaces(replies)
}

func CopyEmptyStructToIfaces(in []struct{}) []interface{} {
	ifaces := make([]interface{}, len(in))
	for i := range in {
		ifaces[i] = &in[i]
	}
	return ifaces
}

func CopyIDsToIfaces(in []*ID) []interface{} {
	ifaces := make([]interface{}, len(in))
	for i := range in {
		in[i] = &ID{}
		ifaces[i] = in[i]
	}
	return ifaces
}