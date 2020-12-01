package main

import (
	"context"
	"errors"
	"log"
	"reflect"
	"time"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/libp2p/go-libp2p-discovery"
	"github.com/libp2p/go-libp2p-gorpc"
)

const (
	bootstrapCount   = 3
	bootstrapTimeout = 5 * time.Second
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

func (c *Cluster) Start() error {
	c.bootstrap()
	err := c.initializeState()
	if err != nil {
		log.Printf("Failed to initialize state: %v", err)
		return err
	}
	return nil
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
					log.Printf("Failed to get peer ID for %q: %-v", p.ID, err)
					continue
				}
				c.peerManager.AddPeer(id)
			}
		}
		time.Sleep(time.Second * 1)
	}
}

func (c *Cluster) bootstrap() bool {
	log.Printf("Waiting until %d peers are discovered", bootstrapCount)

	ticker := time.NewTicker(bootstrapTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			peers := c.peerManager.AllIDs()
			if len(peers) >= bootstrapCount {
				log.Printf("Discovered %d peers", bootstrapCount)
				return true
			}
		}
	}
}

func (c *Cluster) initializeState() error {
	log.Println("Getting state from other peers")

	// Get state from other peers
	peers := c.peerManager.AllIDs().Without(c.id).PeerIDs()
	lenPeers := len(peers)
	ctxts := Ctxts(lenPeers)

	states := make([]*State, lenPeers)

	errs := c.rpcClient.MultiCall(
		ctxts,
		peers,
		ReplicationServiceName,
		ReplicationStateFuncName,
		struct{}{},
		CopyStateToIfaces(states),
	)
	log.Printf("Received state from %d peers, comparing states", lenPeers)

	var statesToCompare []State
	for i, err := range errs {
		if err != nil {
			return err
		}
		statesToCompare = append(statesToCompare, *states[i])
	}

	if len(statesToCompare) > 0 {
		for _, state1 := range states {
			if len(state1.Proposed) == 0 && len(state1.Stored) == 0 {
				continue
			}
			for _, state2 := range states {
				if len(state2.Proposed) == 0 && len(state2.Stored) == 0 {
					continue
				}
				if !Equal(state1.Stored, state2.Stored) || !Equal(state1.Proposed, state2.Proposed) {
					return errors.New("peers disagree on state")
				}
			}
		}

		log.Printf("All states are equal, adopted state")
		c.replicationService.SetState(&statesToCompare[0])
	}
	return nil
}

func (c *Cluster) Peers(ctx context.Context) ([]*ID, error) {
	peers := c.peerManager.AllIDs().PeerIDs()
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

func (c *Cluster) Clusters() map[string][]string {
	clusters := map[string][]string{}

	for _, p := range c.peerManager.peers {
		clusters[p.ClusterID] = append(clusters[p.ClusterID], p.ID.Pretty())
	}
	return clusters
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

func (c *Cluster) RemovePeer(p peer.ID) error {
	log.Printf("Peer %q left", p.Pretty())
	return c.peerManager.RemovePeer(p)
}

func (c *Cluster) Exit() {
	peers := c.peerManager.AllIDs().Without(c.id)
	lenPeers := len(peers)

	replies := make([]struct{}, lenPeers)

	c.rpcClient.MultiCall(
		Ctxts(lenPeers),
		peers.PeerIDs(),
		ClusterServiceName,
		ClusterRemovePeerFuncName,
		c.id,
		CopyEmptyStructToIfaces(replies),
	)
}

func Equal(first map[string][]byte, second map[string][]byte) bool {
	return reflect.DeepEqual(first, second)
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

func CopyStateToIfaces(in []*State) []interface{} {
	ifaces := make([]interface{}, len(in))
	for i := range in {
		in[i] = &State{}
		ifaces[i] = in[i]
	}
	return ifaces
}
