package main

import (
	"context"
	"errors"
	"log"
	"sync"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-gorpc"
)

var (
	ErrDuplicateKey            = errors.New("duplicate key")
	ErrKeyAlreadyProposed      = errors.New("key already proposed")
	ErrProposalNotFound        = errors.New("proposal not found")
	ErrKeyAndValueAlreadyExist = errors.New("key and value already exist")
	ErrNotFound                = errors.New("not found")
)

type ReplicationService struct {
	host      host.Host
	rpcClient *rpc.Client

	store    sync.Map
	proposed sync.Map
}

func NewReplicationService() *ReplicationService {
	return &ReplicationService{
		store:    sync.Map{},
		proposed: sync.Map{},
	}
}

func (r *ReplicationService) Propose(key string, value string) error {
	valueStored, foundStored := r.store.Load(key)

	valueProposed, foundProposed := r.proposed.Load(key)

	if !foundStored && !foundProposed {
		// key does not exist yet
		r.proposed.Store(key, value)
		return nil
	}

	if foundStored && value != valueStored {
		// same key with different value
		return ErrDuplicateKey
	}

	if foundProposed && value != valueProposed {
		return ErrKeyAlreadyProposed
	}

	return ErrKeyAndValueAlreadyExist
}

func (r *ReplicationService) Store(key string, value string) PeerErrors {
	peers := r.host.Peerstore().Peers() // TODO, only connect with our peers, and do clustering
	numberOfPeers := len(peers)

	proposeReplies := make([]ProposeReply, numberOfPeers, numberOfPeers)
	ctxs := make([]context.Context, numberOfPeers, numberOfPeers)
	proposeReplyPointers := make([]interface{}, numberOfPeers, numberOfPeers)

	for i := range proposeReplyPointers {
		proposeReplyPointers[i] = &proposeReplies[i]
		ctxs[i] = context.Background()
	}

	log.Printf("Propose to store key %q at %v", key, peers)
	errs := r.rpcClient.MultiCall(
		ctxs,
		peers,
		ReplicationServiceName,
		ReplicationProposeFuncName,
		ProposeArgs{Key: key, Value: value},
		proposeReplyPointers,
	)
	for _, err := range errs {
		if err != nil {
			return MapPeerErrors(peers, errs)
		}
	}
	log.Printf("All peers accept proposal to store %q", key)

	commitReplies := make([]CommitReply, numberOfPeers, numberOfPeers)
	ctxs = make([]context.Context, numberOfPeers, numberOfPeers)
	commitReplyPointers := make([]interface{}, numberOfPeers, numberOfPeers)

	for i := range commitReplies {
		commitReplyPointers[i] = &commitReplies[i]
		ctxs[i] = context.Background()
	}

	log.Printf("Comitting key %q at %v", key, peers)
	errs = r.rpcClient.MultiCall(
		ctxs,
		peers,
		ReplicationServiceName,
		ReplicationCommitFuncName,
		CommitArgs{Key: key},
		commitReplyPointers,
	)
	for _, err := range errs {
		if err != nil {
			return MapPeerErrors(peers, errs)
		}
	}
	log.Printf("Comitted key %q at %v", key, peers)
	return nil
}

func (r *ReplicationService) StoreLocal(key string, value string) {
	r.store.Store(key, value)
}

func (r *ReplicationService) Commit(key string) error {
	value, found := r.proposed.LoadAndDelete(key)
	if !found {
		log.Printf("Proposal not found: %q", key)
		return ErrProposalNotFound
	}

	r.store.Store(key, value)
	return nil
}

func (r *ReplicationService) GetFromLocal(key string) (string, error) {
	value, found := r.store.Load(key)
	if !found {
		return "", ErrNotFound
	}
	return value.(string), nil
}

func (r *ReplicationService) GetFromPeers(key string) (map[string]string, PeerErrors) {
	peers := r.host.Peerstore().Peers()
	numberOfPeers := len(peers)

	replies := make([]*GetReply, numberOfPeers, numberOfPeers)
	ctxs := make([]context.Context, numberOfPeers, numberOfPeers)

	errs := r.rpcClient.MultiCall(
		ctxs,
		peers,
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
		results[p.String()] = replies[i].Value
	}
	return results, nil
}

// thanks ipfs-cluster
func CopyGetRepliesToIfaces(in []*GetReply) []interface{} {
	ifaces := make([]interface{}, len(in))
	for i := range in {
		in[i] = &GetReply{}
		ifaces[i] = in[i]
	}
	return ifaces
}