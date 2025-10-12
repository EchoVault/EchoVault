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
	"log"

	"github.com/hashicorp/memberlist"
)

type EventDelegate struct {
	options EventDelegateOpts
}

type EventDelegateOpts struct {
	incrementNodes   func()
	decrementNodes   func()
	removeRaftServer func(meta NodeMeta) error
}

func NewEventDelegate(opts EventDelegateOpts) *EventDelegate {
	return &EventDelegate{
		options: opts,
	}
}

// NotifyJoin implements EventDelegate interface
func (eventDelegate *EventDelegate) NotifyJoin(node *memberlist.Node) {
	eventDelegate.options.incrementNodes()
}

// NotifyLeave implements EventDelegate interface
// Note: This is called for both graceful leaves and node failures.
// We do NOT remove the node from Raft here to allow nodes to rejoin after maintenance.
// Raft will handle unreachable nodes through its own timeout mechanisms.
func (eventDelegate *EventDelegate) NotifyLeave(node *memberlist.Node) {
	eventDelegate.options.decrementNodes()

	// Commented out automatic Raft removal to support maintenance scenarios
	// If you need to permanently remove a node, use a manual admin command
	/*
		var meta NodeMeta

		err := json.Unmarshal(node.Meta, &meta)

		if err != nil {
			log.Println("Could not get leaving node's metadata.")
			return
		}

		err = eventDelegate.options.removeRaftServer(meta)

		if err != nil {
			log.Println(err)
		}
	*/

	log.Printf("Node %s left the memberlist (but remains in Raft for potential rejoin)\n", node.Name)
}

// NotifyUpdate implements EventDelegate interface
func (eventDelegate *EventDelegate) NotifyUpdate(node *memberlist.Node) {
	// No-Op
}
