package main

import (
	"bytes"
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

	stored   map[string][]byte
	proposed map[string][]byte
}

func NewReplicationService(ctx context.Context, config Config, peerManager *PeerManager) *ReplicationService {
	return &ReplicationService{
		config:      config,
		peerManager: peerManager,
		stored:      map[string][]byte{},
		proposed:    map[string][]byte{},
	}
}

func (r *ReplicationService) Ready() bool {
	return len(r.peerManager.AllIDs()) >= bootstrapCount
}

func (r *ReplicationService) Store(key string, value []byte) (PeerErrors, error) {
	if !r.Ready() {
		return nil, errors.New("replication service not ready")
	}

	// 1. Propose to our cluster
	log.Printf("Proposing key %q to local cluster", key)
	errs := r.ProposeCluster(key, value)
	if errs != nil {
		return errs, nil
	}
	log.Printf("Successfully proposed key %q to local cluster", key)

	// 2. Propose to all other clusters
	peersPerCluster := r.peerManager.PeersPerCluster()
	for clusterID, peers := range peersPerCluster {
		log.Printf("Proposing to cluster %q", clusterID)
		errs := r.ProposeToPeers(peers, key, value)
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
	err := r.CommitCluster(key)
	if err != nil {
		// TODO clean up proposed key
		return nil, err
	}
	log.Printf("Successfully committed key %q at local cluster", key)

	// 4. Commit the key at every other cluster
	for clusterID, peers := range peersPerCluster {
		log.Printf("Committing key %q at cluster %q", key, clusterID)
		errs := r.CommitAtPeers(peers, key)
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

func (r *ReplicationService) Propose(key string, value []byte) error {
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

func (r *ReplicationService) ProposeCluster(key string, value []byte) PeerErrors {
	peers := r.peerManager.ClusterPeers()
	lenPeers := len(peers)

	replies := make([]struct{}, lenPeers)

	errs := r.rpcClient.MultiCall(
		Ctxts(lenPeers),
		peers.PeerIDs(),
		ReplicationServiceName,
		ReplicationProposeFuncName,
		ProposeArgs{
			Key:   key,
			Value: value,
		},
		CopyEmptyStructToIfaces(replies),
	)
	for _, err := range errs {
		if err != nil {
			return MapPeerErrors(peers, errs)
		}
	}
	return nil
}

func (r *ReplicationService) ProposeToPeers(peers IDs, key string, value []byte) []error {
	var errs []error
	for _, peer := range peers {
		var reply struct{}
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

// StoreLocal is only used from an http endpoint directly
func (r *ReplicationService) StoreLocal(key string, value []byte) {
	r.storeLock.Lock()
	defer r.storeLock.Unlock()

	r.stored[key] = value
}

func (r *ReplicationService) CommitLocal(key string) error {
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

	replies := make([]struct{}, lenPeers)

	errs := r.rpcClient.MultiCall(
		Ctxts(lenPeers),
		peers,
		ReplicationServiceName,
		ReplicationCommitFuncName,
		CommitArgs{
			Key: key,
		},
		CopyEmptyStructToIfaces(replies),
	)
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *ReplicationService) CommitAtPeers(peers IDs, key string) []error {
	var errs []error
	for _, peer := range peers {
		var reply struct{}
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

func (r *ReplicationService) GetLocal(key string) ([]byte, error) {
	r.storeLock.Lock()
	defer r.storeLock.Unlock()

	value, found := r.stored[key]
	if !found {
		return nil, ErrNotFound
	}
	return value, nil
}

func (r *ReplicationService) GetCluster(key string) ([]byte, error) {
	peers := r.peerManager.ClusterPeers()
	lenPeers := len(peers)

	replies := make([]*GetReply, lenPeers)

	errs := r.rpcClient.MultiCall(
		Ctxts(lenPeers),
		peers.PeerIDs(),
		ReplicationServiceName,
		ReplicationGetFuncName,
		GetArgs{Key: key},
		CopyGetRepliesToIfaces(replies),
	)
	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("peer %q err: %w", peers[i].ID.String(), err)
		}
	}

	value := replies[0].Value
	for _, reply := range replies {
		if bytes.Compare(reply.Value, value) != 0 {
			return nil, fmt.Errorf("peers disagree on value for key %q", key)
		}
	}
	return value, nil
}

func (r *ReplicationService) GetFromPeers(peers IDs, key string) ([]byte, []error) {
	var errs []error
	for _, p := range peers {
		var reply GetReply
		err := r.rpcClient.Call(
			p.ID,
			ReplicationServiceName,
			ReplicationGetClusterFuncName,
			GetArgs{
				Key: key,
			},
			&reply,
		)
		if err == nil {
			return reply.Value, nil
		}
		errs = append(errs, err)
	}
	return nil, errs
}

func (r *ReplicationService) GetGlobal(key string) ([]byte, error) {
	// First checkout our cluster
	value, err := r.GetCluster(key)
	if err != nil {
		return nil, err
	}

	// Then check all other clusters
	peersPerCluster := r.peerManager.PeersPerCluster()
	var values = [][]byte{value}

	for clusterID, peers := range peersPerCluster {
		log.Printf("Getting key %q at cluster %q", key, clusterID)
		value, errs := r.GetFromPeers(peers, key)
		for _, err := range errs {
			if err != nil {
				// TODO clean up proposed key and partially committed key
				return nil, fmt.Errorf("failed to get: %-v", err)
			}
		}
		values = append(values, value)
	}

	// Check if clusters agree on values
	for _, v := range values {
		if bytes.Compare(v, value) != 0 {
			return nil, fmt.Errorf("clusters disagree on value")
		}
	}
	return value, nil
}

func (r *ReplicationService) State() (State, error) {
	r.storeLock.Lock()
	defer r.storeLock.Unlock()

	return State{
		Stored:   r.stored,
		Proposed: r.proposed,
	}, nil
}

func (r *ReplicationService) SetState(s *State) {
	r.storeLock.Lock()
	defer r.storeLock.Unlock()

	r.stored = s.Stored
	r.proposed = s.Proposed
}

func CopyGetRepliesToIfaces(out []*GetReply) []interface{} {
	ifaces := make([]interface{}, len(out))
	for i := range out {
		out[i] = &GetReply{}
		ifaces[i] = out[i]
	}
	return ifaces
}

func CopyEmptyStructToIfaces(in []struct{}) []interface{} {
	ifaces := make([]interface{}, len(in))
	for i := range in {
		ifaces[i] = &in[i]
	}
	return ifaces
}
