package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-gorpc"
)

var (
	ErrDuplicateKey       = errors.New("duplicate key")
	ErrKeyAlreadyProposed = errors.New("key already proposed")
	ErrProposalNotFound   = errors.New("proposal not found")
	ErrNotFound           = errors.New("not found")
)

type ReplicationService struct {
	host        host.Host
	rpcClient   *rpc.Client
	config      Config
	peerManager *PeerManager

	storeLock sync.Mutex

	stored   map[string]string
	proposed map[string]string
}

func NewReplicationService(ctx context.Context, config Config, peerManager *PeerManager) *ReplicationService {
	return &ReplicationService{
		config:      config,
		peerManager: peerManager,
		stored:      map[string]string{},
		proposed:    map[string]string{},
	}
}

func (r *ReplicationService) Store(key string, value string) (PeerErrors, error) {

	// 1. Propose to our cluster
	log.Printf("Proposing key %q to local cluster", key)
	err := r.ProposeCluster(key, value)
	if err != nil {
		return nil, err
	}
	log.Printf("Successfully proposed key %q to local cluster", key)

	// 2. Propose to all other clusters
	peersPerCluster := r.peerManager.PeersPerCluster()
	for clusterID, peers := range peersPerCluster {
		log.Printf("Proposing to cluster %q", clusterID)
		errs := r.ProposeOtherCluster(peers, key, value)
		for _, err := range errs {
			if err != nil {
				// TODO clean up proposed key
				log.Printf("Proposal failed: %v", err)
				return MapPeerErrors(peers, errs), nil
			}
		}
	}
	log.Printf("All clusters accepted proposal for key %q", key)

	// 3. Commit the key to our cluster
	log.Printf("Committing %q to local cluster", key)
	err = r.CommitCluster(key)
	if err != nil {
		// TODO clean up proposed key
		return nil, err
	}
	log.Printf("Successfully committed key %q at local cluster", key)

	// 4. Commit the key at every other cluster
	for clusterID, peers := range peersPerCluster {
		log.Printf("Committing key %q at cluster %q", key, clusterID)
		errs := r.CommitOtherCluster(peers, key)
		for _, err := range errs {
			if err != nil {
				// TODO clean up proposed key and partially committed key
				log.Printf("Failed to commit: %-v", err)
				return MapPeerErrors(peers, errs), nil
			}
		}
	}
	log.Printf("Successfully committed key %q at all clusters", key)
	return nil, nil
}

func (r *ReplicationService) Propose(key string, value string) error {
	r.storeLock.Lock()
	defer r.storeLock.Unlock()

	_, foundStored := r.stored[key]

	_, foundProposed := r.proposed[key]

	if !foundStored && !foundProposed {
		// key does not exist yet
		r.proposed[key] = value
		return nil
	}

	if foundStored {
		return ErrDuplicateKey
	}

	if foundProposed {
		return ErrKeyAlreadyProposed
	}

	return nil
}

func (r *ReplicationService) ProposeCluster(key string, value string) error {
	peers := r.peerManager.ClusterPeers().PeerIDs()
	lenPeers := len(peers)

	proposeReplies := make([]*ProposeReply, lenPeers)

	errs := r.rpcClient.MultiCall(
		Ctxts(lenPeers),
		peers,
		ReplicationServiceName,
		ReplicationProposeFuncName,
		ProposeArgs{
			Key:   key,
			Value: value,
		},
		CopyProposeRepliesToIfaces(proposeReplies),
	)
	for i, err := range errs {
		if err != nil {
			return fmt.Errorf("key %q rejected by %q: %w", key, peers[i], err)
		}
	}
	return nil
}

func (r *ReplicationService) ProposeOtherCluster(peers IDs, key string, value string) []error {
	var errs []error
	for _, peer := range peers {
		var reply ProposeReply
		err := r.rpcClient.Call(
			peer.ID,
			ReplicationServiceName,
			ReplicationProposeClusterFuncName,
			ProposeArgs{
				Key:   key,
				Value: value,
			},
			&reply,
		)
		if err == nil {
			// Success
			return nil
		}
		errs = append(errs, err)
	}
	return errs
}

func (r *ReplicationService) StoreLocal(key string, value string) {
	r.storeLock.Lock()
	defer r.storeLock.Unlock()

	r.stored[key] = value
}

func (r *ReplicationService) Commit(key string) error {
	r.storeLock.Lock()
	defer r.storeLock.Unlock()

	value, found := r.proposed[key]
	if !found {
		log.Printf("Proposal not found: %q", key)
		return ErrProposalNotFound
	}
	delete(r.proposed, key)

	r.stored[key] = value
	return nil
}

func (r *ReplicationService) CommitCluster(key string) error {
	peers := r.peerManager.ClusterPeers().PeerIDs()
	lenPeers := len(peers)

	replies := make([]*CommitReply, lenPeers)

	errs := r.rpcClient.MultiCall(
		Ctxts(lenPeers),
		peers,
		ReplicationServiceName,
		ReplicationCommitFuncName,
		CommitArgs{
			Key: key,
		},
		CopyCommitRepliesToIfaces(replies),
	)
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *ReplicationService) CommitOtherCluster(peers IDs, key string) []error {
	var errs []error
	for _, peer := range peers {
		var reply CommitReply
		err := r.rpcClient.Call(
			peer.ID,
			ReplicationServiceName,
			ReplicationCommitClusterFuncName,
			CommitArgs{
				Key: key,
			},
			&reply,
		)
		if err == nil {
			return nil
		}
		errs = append(errs, err)
	}
	return errs
}

func (r *ReplicationService) GetLocal(key string) (string, error) {
	r.storeLock.Lock()
	defer r.storeLock.Unlock()

	value, found := r.stored[key]
	if !found {
		return "", ErrNotFound
	}
	return value, nil
}

func (r *ReplicationService) GetGlobal(key string) (map[string]string, PeerErrors) {
	peers := r.peerManager.AllIDs()
	lenPeers := len(peers)

	replies := make([]*GetReply, lenPeers, lenPeers)

	errs := r.rpcClient.MultiCall(
		Ctxts(lenPeers),
		peers.PeerIDs(),
		ReplicationServiceName,
		ReplicationGetFuncName,
		GetArgs{Key: key},
		CopyGetRepliesToIfaces(replies),
	)
	for _, err := range errs {
		if err != nil {
			return nil, MapPeerErrors(peers, errs)
		}
	}

	var results = map[string]string{}
	for i, p := range peers {
		results[p.ID.String()] = replies[i].Value
	}
	return results, nil
}

func CopyGetRepliesToIfaces(out []*GetReply) []interface{} {
	ifaces := make([]interface{}, len(out))
	for i := range out {
		out[i] = &GetReply{}
		ifaces[i] = out[i]
	}
	return ifaces
}

func CopyProposeRepliesToIfaces(out []*ProposeReply) []interface{} {
	ifaces := make([]interface{}, len(out))
	for i := range out {
		out[i] = &ProposeReply{}
		ifaces[i] = out[i]
	}
	return ifaces
}

func CopyCommitRepliesToIfaces(out []*CommitReply) []interface{} {
	ifaces := make([]interface{}, len(out))
	for i := range out {
		out[i] = &CommitReply{}
		ifaces[i] = out[i]
	}
	return ifaces
}
