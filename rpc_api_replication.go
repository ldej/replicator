package main

import (
	"context"
	"log"
)

const (
	ReplicationServiceName     = "ReplicationRPCAPI"
	ReplicationProposeFuncName = "Propose"
	ReplicationCommitFuncName  = "Commit"
	ReplicationGetFuncName     = "Get"
)

type ReplicationRPCAPI struct {
	r *ReplicationService
}

type ProposeArgs struct {
	Key string
	Value string
}

type ProposeReply struct {}

func (r *ReplicationRPCAPI) Propose(ctx context.Context, argType ProposeArgs, replyType *ProposeReply) error {
	log.Printf("Received proposal for key: %q", argType.Key)
	return r.r.Propose(argType.Key, argType.Value)
}

type CommitArgs struct {
	Key string
}

type CommitReply struct {}

func (r *ReplicationRPCAPI) Commit(ctx context.Context, argType CommitArgs, replyType *CommitReply) error {
	log.Printf("Received commit for key: %q", argType.Key)
	return r.r.Commit(argType.Key)
}


type GetArgs struct {
	Key string
}

type GetReply struct {
	Value string
}

func (r *ReplicationRPCAPI) Get(ctx context.Context, argType GetArgs, replyType *GetReply) error {
	value, err := r.r.GetFromLocal(argType.Key)
	if err != nil {
		return err
	}
	replyType.Value = value
	return nil
}
