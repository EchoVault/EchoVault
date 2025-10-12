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
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/echovault/sugardb/internal"
	"github.com/echovault/sugardb/internal/config"
	"github.com/hashicorp/memberlist"
	"github.com/hashicorp/raft"
)

type Delegate struct {
	options DelegateOpts
}

type DelegateOpts struct {
	config         config.Config
	broadcastQueue *memberlist.TransmitLimitedQueue
	addVoter       func(id raft.ServerID, address raft.ServerAddress, prevIndex uint64, timeout time.Duration) error
	isRaftLeader   func() bool
	applyMutate    func(ctx context.Context, cmd []string) ([]byte, error)
	applyDeleteKey func(ctx context.Context, key string) error
}

func NewDelegate(opts DelegateOpts) *Delegate {
	return &Delegate{
		options: opts,
	}
}

// NodeMeta implements Delegate interface
func (delegate *Delegate) NodeMeta(limit int) []byte {
	meta := NodeMeta{
		ServerID: raft.ServerID(delegate.options.config.ServerID),
		RaftAddr: raft.ServerAddress(
			fmt.Sprintf("%s:%d", delegate.options.config.RaftAdvertiseAddr, delegate.options.config.RaftAdvertisePort)),
		MemberlistAddr: fmt.Sprintf("%s:%d", delegate.options.config.AdvertiseAddr, delegate.options.config.DiscoveryPort),
	}

	b, err := json.Marshal(&meta)

	if err != nil {
		return []byte("")
	}

	return b
}

// NotifyMsg implements Delegate interface
func (delegate *Delegate) NotifyMsg(msgBytes []byte) {
	var msg BroadcastMessage
	if err := json.Unmarshal(msgBytes, &msg); err != nil {
		log.Printf("notifymsg: %v", err)
		return
	}

	switch msg.Action {
	case "RaftJoin":
		// If the current node is not the cluster leader, re-broadcast the message.
		if !delegate.options.isRaftLeader() {
			delegate.options.broadcastQueue.QueueBroadcast(&msg)
			return
		}
		err := delegate.options.addVoter(msg.NodeMeta.ServerID, msg.NodeMeta.RaftAddr, 0, 0)
		if err != nil {
			log.Println(err)
		}

	case "DeleteKey":
		// If the current node is not a cluster leader, re-broadcast the message.
		if !delegate.options.isRaftLeader() {
			delegate.options.broadcastQueue.QueueBroadcast(&msg)
			return
		}
		// Current node is the cluster leader, handle the key deletion.
		ctx := context.WithValue(
			context.WithValue(context.Background(), internal.ContextServerID("ServerID"), string(msg.ServerID)),
			internal.ContextConnID("ConnectionID"), msg.ConnId)

		key := string(msg.Content)

		if err := delegate.options.applyDeleteKey(ctx, key); err != nil {
			log.Println(err)
		}

	case "MutateData":
		// If the current node is not a cluster leader, re-broadcast the message.
		if !delegate.options.isRaftLeader() {
			delegate.options.broadcastQueue.QueueBroadcast(&msg)
			return
		}
		// Current node is the cluster leader, handle the mutation
		ctx := context.WithValue(
			context.WithValue(context.Background(), internal.ContextServerID("ServerID"), string(msg.ServerID)),
			internal.ContextConnID("ConnectionID"), msg.ConnId)

		cmd, err := internal.Decode(msg.Content)
		if err != nil {
			log.Println(err)
			return
		}

		if _, err := delegate.options.applyMutate(ctx, cmd); err != nil {
			log.Println(err)
		}
	}
}

// GetBroadcasts implements Delegate interface
func (delegate *Delegate) GetBroadcasts(overhead, limit int) [][]byte {
	return delegate.options.broadcastQueue.GetBroadcasts(overhead, limit)
}

// LocalState implements Delegate interface
func (delegate *Delegate) LocalState(join bool) []byte {
	// No-Op
	return []byte("")
}

// MergeRemoteState implements Delegate interface
func (delegate *Delegate) MergeRemoteState(buf []byte, join bool) {
	// No-Op
}
