package main

import (
	"context"
	"flag"
	"log"
	"strings"

	"github.com/multiformats/go-multiaddr"
)

type Config struct {
	HttpPort        int
	ClusterID       string
	ProtocolID      string
	Rendezvous      string
	DiscoveryPeers  addrList
	BootstrapPeers  addrList
	ListenAddresses addrList
}

type addrList []multiaddr.Multiaddr

func (al *addrList) String() string {
	strs := make([]string, len(*al))
	for i, addr := range *al {
		strs[i] = addr.String()
	}
	return strings.Join(strs, ",")
}

func (al *addrList) Set(value string) error {
	addr, err := multiaddr.NewMultiaddr(value)
	if err != nil {
		return err
	}
	*al = append(*al, addr)
	return nil
}

func main() {
	config := Config{}

	flag.StringVar(&config.Rendezvous, "rendezvous", "ldej/replicator", "")
	flag.Var(&config.DiscoveryPeers, "peer", "Peer multiaddress for peer discovery")
	flag.Var(&config.BootstrapPeers, "bootstrap", "Peer multiaddress for joining a cluster")
	flag.StringVar(&config.ClusterID, "cid", "", "ID of the cluster to join")
	flag.Var(&config.ListenAddresses, "listen", "Adds a multiaddress to the listen list")
	flag.StringVar(&config.ProtocolID, "protocolid", "/p2p/rpc/replicator", "")
	flag.IntVar(&config.HttpPort, "http", 8000, "")
	flag.Parse()

	ctx := context.Background()

	host := NewHost(ctx, config)

	peerManager := NewPeerManager(ctx, config)

	replicationService := NewReplicationService(ctx, config, peerManager)

	cluster, err := NewCluster(ctx, host, config, replicationService, peerManager)
	if err != nil {
		log.Fatal(err)
	}

	StartWebServer(config.HttpPort, cluster)
}
