package main

import (
	"context"
	"log"
	"sync"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	disc "github.com/libp2p/go-libp2p-discovery"
	"github.com/libp2p/go-libp2p-kad-dht"
)

func NewKDHT(ctx context.Context, host host.Host, config Config) (*disc.RoutingDiscovery, error) {
	kdht, err := dht.New(ctx, host)
	if err != nil {
		return nil, err
	}

	log.Println("Bootstrapping the DHT")
	if err = kdht.Bootstrap(ctx); err != nil {
		return nil, err
	}

	// Let's connect to the bootstrap nodes first. They will tell us about the
	// other nodes in the network.
	var wg sync.WaitGroup
	for _, peerAddr := range config.BootstrapPeers {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := host.Connect(ctx, *peerinfo); err != nil {
				log.Println(err)
			} else {
				log.Println("Connection established with bootstrap node:", *peerinfo)
			}
		}()
	}
	wg.Wait()
	routingDiscovery := disc.NewRoutingDiscovery(kdht)
	return routingDiscovery, nil
}