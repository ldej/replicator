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

const (
	bootstrapMinPeers = 3

	maxPeersCluster = 5
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

			log.Printf("Connected to %s", p.ID)

			c.addPeer(p.ID)

			if len(c.clusterPeers) == 0 {
				c.joinOrFormCluster(p.ID)
			}
		}
		time.Sleep(time.Second * 15)
	}
}

func (c *Cluster) joinOrFormCluster(newPeer peer.ID) {
	// Check if we can join the peer
	err := c.rpcClient.Call(newPeer, ClusterServiceName, ClusterAddPeerFuncName, c.id, &struct{}{})
	if err == nil {
		return
	}
	log.Printf("Cannot join %s: %v", newPeer.String(), err)

	// there are not enough peers to bootstrap
	if len(c.allPeers) < bootstrapMinPeers {
		log.Printf("not enough peers to bootstrap: %d", len(c.allPeers))
		return
	}

	log.Printf("we are bootstrapping!")
	// if we are still not in a cluster, we are bootstrapping
	c.bootstrapping = true

	// If I'm the first peer, ask the other peers to join me
	if c.allPeers[0] != c.id {
		log.Printf("I'm not the first peer")
		return
	}
	log.Printf("I'm the first peer!")

	members := c.allPeers
	errs := c.rpcClient.MultiCall(
		Ctxts(len(members)),
		members,
		ClusterServiceName,
		ClusterJoinFuncName,
		c.id,
		RPCDiscardReplies(len(members)),
	)
	for i, err := range errs {
		if err != nil {
			log.Printf("Peer %s returned err: %-v", members[i], err)
		}
	}
	log.Printf("Everybody joined me!")
	c.bootstrapping = false
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

	c.rpcClient.MultiCall(
		Ctxts(len(c.clusterPeers)),
		c.clusterPeers,
		ClusterServiceName,
		ClusterUpdatePeersFuncName,
		c.clusterPeers,
		RPCDiscardReplies(len(c.clusterPeers)),
	)
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

	if !c.isLeader() {
		log.Printf("I'm not the leader, forwarding!")
		return c.rpcClient.Call(c.clusterPeers[0], ClusterServiceName, ClusterAddPeerFuncName, p, &struct{}{})
	} else {
		log.Printf("I am the leader, let me add you")
	}

	if len(c.clusterPeers) == maxPeersCluster {
		log.Printf("The cluster is full")
		// Put the requesting node in bootstrap mode
		err := c.rpcClient.Call(p, ClusterServiceName, ClusterBootstrapFuncName, nil, &struct{}{})
		if err != nil {
			log.Printf("Bootstrap call at peer %s failed: %-v", p, err)
			return nil
		}

		var leavingPeers peer.IDSlice
		c.clusterPeers, leavingPeers = c.clusterPeers[:len(c.clusterPeers)-2], c.clusterPeers[len(c.clusterPeers)-2:]

		errs := c.rpcClient.MultiCall(
			Ctxts(len(leavingPeers)),
			leavingPeers,
			ClusterServiceName,
			ClusterJoinFuncName,
			p,
			RPCDiscardReplies(len(leavingPeers)),
		)
		for i, err := range errs {
			if err != nil {
				log.Printf("Update peers lef: Peer %s err: %v", leavingPeers[i], err)
			}
		}
		errs = c.rpcClient.MultiCall(
			Ctxts(len(c.clusterPeers)),
			c.clusterPeers,
			ClusterServiceName,
			ClusterUpdatePeersFuncName,
			c.clusterPeers,
			RPCDiscardReplies(len(c.clusterPeers)),
		)
		for i, err := range errs {
			if err != nil {
				log.Printf("Update remaining peers: Peer %s err: %v", c.clusterPeers[i], err)
			}
		}
		return nil
	}

	c.addClusterPeer(p)
	return nil
}

func (c *Cluster) Peers(ctx context.Context) (peer.IDSlice, error) {
	if c.isLeader() {
		return c.clusterPeers, nil
	}
	// TODO forward to leader ?
	return nil, ErrNotLeader
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
	log.Printf("Joined cluster %s, getting peers", p.String())
	// if we joined a cluster, get the peers
	var peers peer.IDSlice
	err = c.rpcClient.Call(p, ClusterServiceName, ClusterPeersFuncName, nil, &peers)
	if err != nil {
		log.Printf("Failed to get peers: %-v", err)
		return err
	}
	log.Printf("My cluster peers are: %v", peers)
	c.clusterPeers = peers
	return nil
}

func (c *Cluster) isLeader() bool {
	if len(c.clusterPeers) == 0 {
		return false
	}
	sort.Sort(c.clusterPeers)
	return c.clusterPeers[0] == c.id
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