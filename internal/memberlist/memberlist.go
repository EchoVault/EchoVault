// Copyright 2024 Kelvin Clement Mwinuka
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package memberlist

import (
	"context"
	"crypto/md5"
	"fmt"
	"github.com/echovault/echovault/internal"
	"github.com/echovault/echovault/internal/config"
	"log"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
	"github.com/hashicorp/raft"
	"github.com/sethvargo/go-retry"
)

type NodeMeta struct {
	ServerID       raft.ServerID      `json:"ServerID"`
	MemberlistAddr string             `json:"MemberlistAddr"`
	RaftAddr       raft.ServerAddress `json:"RaftAddr"`
}

type Opts struct {
	Config           config.Config
	HasJoinedCluster func() bool
	AddVoter         func(id raft.ServerID, address raft.ServerAddress, prevIndex uint64, timeout time.Duration) error
	RemoveRaftServer func(meta NodeMeta) error
	IsRaftLeader     func() bool
	ApplyMutate      func(ctx context.Context, cmd []string) ([]byte, error)
	ApplyDeleteKey   func(ctx context.Context, key string) error
}

type MemberList struct {
	options        Opts
	broadcastQueue *memberlist.TransmitLimitedQueue
	noOfNodesMut   sync.RWMutex
	noOfNodes      int
	memberList     *memberlist.Memberlist
}

func NewMemberList(opts Opts) *MemberList {
	return &MemberList{
		options:        opts,
		broadcastQueue: new(memberlist.TransmitLimitedQueue),
		noOfNodesMut:   sync.RWMutex{},
		noOfNodes:      0,
	}
}

func (m *MemberList) MemberListInit(ctx context.Context) {
	cfg := memberlist.DefaultWANConfig()
	cfg.RequireNodeNames = true
	cfg.Name = m.options.Config.ServerID
	cfg.BindAddr = m.options.Config.BindAddr
	cfg.BindPort = int(m.options.Config.DiscoveryPort)
	cfg.Delegate = NewDelegate(DelegateOpts{
		config:         m.options.Config,
		broadcastQueue: m.broadcastQueue,
		addVoter:       m.options.AddVoter,
		isRaftLeader:   m.options.IsRaftLeader,
		applyMutate:    m.options.ApplyMutate,
		applyDeleteKey: m.options.ApplyDeleteKey,
	})
	cfg.Events = NewEventDelegate(EventDelegateOpts{
		incrementNodes: func() {
			m.noOfNodesMut.Lock()
			defer m.noOfNodesMut.Unlock()
			m.noOfNodes += 1
		},
		decrementNodes: func() {
			m.noOfNodesMut.Lock()
			defer m.noOfNodesMut.Unlock()
			m.noOfNodes -= 1
		},
		removeRaftServer: m.options.RemoveRaftServer,
	})

	m.broadcastQueue.RetransmitMult = 1
	m.broadcastQueue.NumNodes = func() int {
		m.noOfNodesMut.RLock()
		defer m.noOfNodesMut.RUnlock()
		noOfNodes := m.noOfNodes
		return noOfNodes
	}

	list, err := memberlist.Create(cfg)
	m.memberList = list

	if err != nil {
		log.Fatal(err)
	}

	if m.options.Config.JoinAddr != "" {
		backoffPolicy := internal.RetryBackoff(retry.NewFibonacci(1*time.Second), 5, 200*time.Millisecond, 0, 0)

		err = retry.Do(ctx, backoffPolicy, func(ctx context.Context) error {
			_, err = list.Join([]string{m.options.Config.JoinAddr})
			if err != nil {
				return retry.RetryableError(err)
			}
			return nil
		})

		if err != nil {
			log.Fatal(err)
		}

		m.broadcastRaftAddress()
	}
}

func (m *MemberList) broadcastRaftAddress() {
	msg := BroadcastMessage{
		Action: "RaftJoin",
		NodeMeta: NodeMeta{
			ServerID: raft.ServerID(m.options.Config.ServerID),
			RaftAddr: raft.ServerAddress(fmt.Sprintf("%s:%d",
				m.options.Config.RaftBindAddr, m.options.Config.RaftBindPort)),
		},
	}
	m.broadcastQueue.QueueBroadcast(&msg)
}

// The ForwardDeleteKey function is only called by non-leaders.
// It uses the broadcast queue to forward a key eviction command within the cluster.
func (m *MemberList) ForwardDeleteKey(ctx context.Context, key string) {
	connId, _ := ctx.Value(internal.ContextConnID("ConnectionID")).(string)
	m.broadcastQueue.QueueBroadcast(&BroadcastMessage{
		Action:      "DeleteKey",
		Content:     []byte(key),
		ContentHash: md5.Sum([]byte(key)),
		ConnId:      connId,
		NodeMeta: NodeMeta{
			ServerID: raft.ServerID(m.options.Config.ServerID),
			RaftAddr: raft.ServerAddress(fmt.Sprintf("%s:%d",
				m.options.Config.BindAddr, m.options.Config.RaftBindPort)),
		},
	})
}

// The ForwardDataMutation function is only called by non-leaders.
// It uses the broadcast queue to forward a data mutation within the cluster.
func (m *MemberList) ForwardDataMutation(ctx context.Context, cmd []byte) {
	connId, _ := ctx.Value(internal.ContextConnID("ConnectionID")).(string)
	m.broadcastQueue.QueueBroadcast(&BroadcastMessage{
		Action:      "MutateData",
		Content:     cmd,
		ContentHash: md5.Sum(cmd),
		ConnId:      connId,
		NodeMeta: NodeMeta{
			ServerID: raft.ServerID(m.options.Config.ServerID),
			RaftAddr: raft.ServerAddress(fmt.Sprintf("%s:%d",
				m.options.Config.BindAddr, m.options.Config.RaftBindPort)),
		},
	})
}

func (m *MemberList) MemberListShutdown() {
	// Gracefully leave memberlist cluster
	err := m.memberList.Leave(500 * time.Millisecond)
	if err != nil {
		log.Printf("memberlist leave: %v\n", err)
		return
	}

	err = m.memberList.Shutdown()
	if err != nil {
		log.Printf("memberlist shutdown: %v\n", err)
		return
	}

	log.Println("successfully shutdown memberlist")
}
