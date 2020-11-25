# Work-in-progress: Replicator

## Problem statement

Design, develop and deploy a multi-cluster key-value replication engine using golang and libp2p. The network can be split into small clusters of 3 or more nodes, with each cluster given a unique cluster ID and each peer given a unique peer Id.

A client proposes a key-value by making an RPC call to any of the node in any of the clusters.

The node which receives this request should replicate the key-value entry across all the nodes in the current cluster and also send it across the remaining clusters.

In case of conflicts with the proposed key in any of the clusters(key already exists), client should be prompted to change the key.

Every node should be capable of handling the RPC requests sent by clients in the network.

Client should also be able to retrieve the value associated with a given key by making an RPC call.

Your model should be capable of handling concurrent requests across multiple clusters.

## Design

### Replication

#### Store happy path

1. Propose to store to all peers within the cluster and to representatives in other clusters
2. Each peer marks the key as reserved
3. Each peer only confirms when key is not stored or reserved
4. All peer confirm
5. Peer sends commit command
6. All clients confirm commit

#### Key already stored

1. Propose to store to all peers within the cluster and in other clusters
2. A peer denies because a key is already stored
3. Initiating peer cleans up where the key is marked as reserved already

#### Key already reserved

1. Propose to store to all clients within the cluster and in other clusters
2. A client denies because a key is already reserved
3. Initiating peer cleans up where the key is marked as stored already

#### Request value for key

1. Get key from any client within the cluster or any other peer in other clusters
2. Return value for key

#### Requested key doesn't exist

1. Get key from any client within the cluster or other clusters
2. Return unknown key error

### Two phase commit

When storing a key, a peer will consult the peers in its cluster, and 1 representative in each cluster.
The representative in a cluster consults with the peers in its cluster and returns the response to the requesting peer. 

A peer treats itself as a peer, so it communicates with its own rpc server.

### DHT Bootstrapping

One peer is started as the DHT bootstrap peer. The other peers will connect to the bootstrap peer. If we cannot connect to the DHT bootstrap node, then we cannot continue.

### Clustering

A peer is started with a cluster ID.

## Instructions

```shell script
$ git clone git@github.com:ldej/replicator.git
$ cd replicator
$ go build
```

```shell script
$ ./replicator --help
Usage of ./replicator:
  -bootstrap value
    	Peer multiaddress for joining a cluster
  -cid string
    	ID of the cluster to join
  -http int
    	
  -listen value
    	Adds a multiaddress to the listen list
  -peer value
    	Peer multiaddress for peer discovery
  -protocolid string
    	 (default "/p2p/rpc/replicator")
  -rendezvous string
    	 (default "ldej/replicator")
```

Start one node with:
```shell script
$ ./replicator -cid cluster-1 -http 8000
Host ID: QmSxMCdCjcC3hJXVWnsdBkLF6jFC9KEs9cQerRndf7Arbo
Connect to me on:
 - /ip6/::1/tcp/36177/p2p/QmSxMCdCjcC3hJXVWnsdBkLF6jFC9KEs9cQerRndf7Arbo
 - /ip4/192.168.1.8/tcp/39307/p2p/QmSxMCdCjcC3hJXVWnsdBkLF6jFC9KEs9cQerRndf7Arbo
 - /ip4/127.0.0.1/tcp/39307/p2p/QmSxMCdCjcC3hJXVWnsdBkLF6jFC9KEs9cQerRndf7Arbo
Waiting until 3 peers are discovered
```
This will be the peer that starts the DHT.

Start at least two other nodes with:
```shell script
$ ./replicator -cid cluster-1 -peer /ip4/192.168.1.8/tcp/39307/p2p/QmSxMCdCjcC3hJXVWnsdBkLF6jFC9KEs9cQerRndf7Arbo
```

You can choose an HTTP port, if not, it takes an available port and log which port it has taken.

## Improvements

- Check if proposal and commit are from the same peer?
- Resolve disagreements between peers

## Tech

https://github.com/libp2p/go-libp2p

https://github.com/libp2p/go-libp2p-gorpc

https://github.com/libp2p/go-libp2p-kad-dht

GOPRIVATE='github.com/libp2p/*' go get ./...

# TODO

- detect disconnected peers
- clean up proposals and commits when a failure happens when storing