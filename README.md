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

1. Propose to store to all clients within the cluster and in other clusters
2. Each client marks the key as reserved
3. Each client only confirms when key is not stored or reserved
4. All clients confirm
5. Client sends store command
6. All clients confirm store

#### Key already stored

1. Propose to store to all clients within the cluster and in other clusters
2. A client denies because a key is already stored
3. Initiating client sends DECLINE to all clients within the same cluster and other clusters

#### Key already reserved

1. Propose to store to all clients within the cluster and in other clusters
2. A client denies because a key is already reserved
3. Initiating client sends DECLINE to all clients within the same cluster and other clusters

#### Request value for key happy path

1. Get key from any client within the cluster or other clusters
2. Return value for key

#### Requested key doesn't exist

1. Get key from any client within the cluster or other clusters
2. Return unknown key

### Considerations

When storing a key, a peer will consult the peers in its cluster, and 1 representative in each cluster.
The representative in a cluster consults with the peers in its cluster and returns the response to the requesting peer. 

A peer treats itself as a peer, so it communicates with its own rpc server.

### DHT Bootstrapping

One peer is started as the DHT bootstrap peer. The other peers will connect to the bootstrap peer. If we cannot connect to the DHT bootstrap node, then we cannot continue.

### Clustering

A peer is started with a cluster ID.

## Tech

### libp2p

https://github.com/libp2p/go-libp2p

https://github.com/libp2p/go-libp2p-gorpc

https://github.com/libp2p/go-libp2p-kad-dht

GOPRIVATE='github.com/libp2p/*' go get ./...

## Instructions

TODO

## Improvements

- Check if proposal and commit are from the same peer?
- Resolve disagreements between peers

# TODO

- instructions
- string to []byte
- replicate state and metadata when node joins cluster
- clean up proposals and commits when a failure happens when storing