package main

import (
	"context"
	"log"
)

const (
	ReplicationServiceName            = "ReplicationRPCAPI"
	ReplicationProposeFuncName        = "Propose"
	ReplicationProposeClusterFuncName = "ProposeCluster"
	ReplicationCommitFuncName         = "Commit"
	ReplicationCommitClusterFuncName  = "CommitCluster"
	ReplicationGetFuncName            = "Get"
	ReplicationGetClusterFuncName     = "GetCluster"
	ReplicationStateFuncName          = "State"
)

type ReplicationRPCAPI struct {
	r *ReplicationService
}

type ProposeArgs struct {
	Key   string
	Value []byte
}

type ProposeReply struct {
	Errors PeerErrors
}

func (r *ReplicationRPCAPI) Propose(ctx context.Context, in ProposeArgs, out *struct{}) error {
	log.Printf("Received proposal for key: %q", in.Key)
	return r.r.Propose(in.Key, in.Value)
}

func (r *ReplicationRPCAPI) ProposeCluster(ctx context.Context, in ProposeArgs, out *ProposeReply) error {
	log.Printf("Received proposal cluster for key: %q", in.Key)
	out.Errors = r.r.ProposeCluster(in.Key, in.Value)
	return nil
}

type CommitArgs struct {
	Key string
}

func (r *ReplicationRPCAPI) Commit(ctx context.Context, in CommitArgs, out *struct{}) error {
	log.Printf("Received commit for key: %q", in.Key)
	return r.r.CommitLocal(in.Key)
}

func (r *ReplicationRPCAPI) CommitCluster(ctx context.Context, in CommitArgs, out *struct{}) error {
	log.Printf("Received commit cluster for key: %q", in.Key)
	return r.r.CommitCluster(in.Key)
}

type GetArgs struct {
	Key string
}

type GetReply struct {
	Value []byte
}

func (r *ReplicationRPCAPI) Get(ctx context.Context, in GetArgs, out *GetReply) error {
	value, err := r.r.GetLocal(in.Key)
	if err != nil {
		return err
	}
	out.Value = value
	return nil
}

func (r *ReplicationRPCAPI) GetCluster(ctx context.Context, in GetArgs, out *GetReply) error {
	value, err := r.r.GetCluster(in.Key)
	if err != nil {
		return err
	}
	out.Value = value
	return nil
}

type State struct {
	Stored   map[string][]byte
	Proposed map[string][]byte
}

func (r *ReplicationRPCAPI) State(ctx context.Context, in struct{}, out *State) error {
	state, err := r.r.State()
	if err != nil {
		return err
	}
	*out = state
	return nil
}
