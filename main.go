package main

import (
	"context"
	"flag"
	"log"
	"strings"

	"github.com/multiformats/go-multiaddr"
)

type Config struct {
	HttpPort int
	ProtocolID string
	Rendezvous string
	BootstrapPeers addrList
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

	flag.StringVar(&config.Rendezvous, "rendezvous", "ldej/testing", "")
	flag.Var(&config.BootstrapPeers, "peer", "Peer multiaddress for bootstrapping")
	flag.Var(&config.ListenAddresses, "listen", "Adds a multiaddress to the listen list")
	flag.StringVar(&config.ProtocolID, "protocolid", "/p2p/rpc/replicator", "")
	flag.IntVar(&config.HttpPort, "http", 8000, "")
	flag.Parse()

	// if len(config.BootstrapPeers) == 0 {
	// 	config.BootstrapPeers = dht.DefaultBootstrapPeers
	// }

	ctx := context.Background()

	host := NewHost(ctx, config)

	replicationService := NewReplicationService()

	cluster, err := NewCluster(ctx, host, config, replicationService)
	if err != nil {
		log.Fatal(err)
	}

	StartWebServer(config.HttpPort, cluster)
}