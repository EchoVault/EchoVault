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

package raft

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/echovault/echovault/internal"
	"github.com/echovault/echovault/internal/config"
	"github.com/hashicorp/raft"
	"io"
	"log"
	"net"
	"strings"
	"time"
)

type FSMOpts struct {
	Config                config.Config
	GetState              func() map[int]map[string]internal.KeyData
	GetCommand            func(command string) (internal.Command, error)
	SetValues             func(ctx context.Context, entries map[string]interface{}) error
	SetExpiry             func(ctx context.Context, key string, expire time.Time, touch bool)
	DeleteKey             func(ctx context.Context, key string) error
	StartSnapshot         func()
	FinishSnapshot        func()
	SetLatestSnapshotTime func(msec int64)
	GetHandlerFuncParams  func(ctx context.Context, cmd []string, conn *net.Conn) internal.HandlerFuncParams
}

type FSM struct {
	options FSMOpts
}

func NewFSM(opts FSMOpts) raft.FSM {
	return raft.FSM(&FSM{
		options: opts,
	})
}

// Apply Implements raft.FSM interface
func (fsm *FSM) Apply(log *raft.Log) interface{} {
	switch log.Type {
	default:
		// No-Op
	case raft.LogCommand:
		var request internal.ApplyRequest

		if err := json.Unmarshal(log.Data, &request); err != nil {
			return internal.ApplyResponse{
				Error:    err,
				Response: nil,
			}
		}

		ctx := context.WithValue(context.Background(), internal.ContextServerID("ServerID"), request.ServerID)
		ctx = context.WithValue(ctx, internal.ContextConnID("ConnectionID"), request.ConnectionID)
		ctx = context.WithValue(ctx, "Protocol", request.Protocol)
		ctx = context.WithValue(ctx, "Database", request.Database)

		switch strings.ToLower(request.Type) {
		default:
			return internal.ApplyResponse{
				Error:    fmt.Errorf("unsupported raft command type %s", request.Type),
				Response: nil,
			}

		case "delete-key":
			if err := fsm.options.DeleteKey(ctx, request.Key); err != nil {
				return internal.ApplyResponse{
					Error:    err,
					Response: nil,
				}
			}
			return internal.ApplyResponse{
				Error:    nil,
				Response: []byte("OK"),
			}

		case "command":
			// Handle command
			command, err := fsm.options.GetCommand(request.CMD[0])
			if err != nil {
				return internal.ApplyResponse{
					Error:    err,
					Response: nil,
				}
			}

			handler := command.HandlerFunc

			sc, err := internal.GetSubCommand(command, request.CMD)
			if err != nil {
				return internal.ApplyResponse{
					Error:    err,
					Response: nil,
				}
			}
			subCommand, ok := sc.(internal.SubCommand)
			if ok {
				handler = subCommand.HandlerFunc
			}

			if res, err := handler(fsm.options.GetHandlerFuncParams(ctx, request.CMD, nil)); err != nil {
				return internal.ApplyResponse{
					Error:    err,
					Response: nil,
				}
			} else {
				return internal.ApplyResponse{
					Error:    nil,
					Response: res,
				}
			}
		}
	}

	return nil
}

// Snapshot implements raft.FSM interface
func (fsm *FSM) Snapshot() (raft.FSMSnapshot, error) {
	return NewFSMSnapshot(SnapshotOpts{
		config:                fsm.options.Config,
		startSnapshot:         fsm.options.StartSnapshot,
		finishSnapshot:        fsm.options.FinishSnapshot,
		setLatestSnapshotTime: fsm.options.SetLatestSnapshotTime,
		data:                  fsm.options.GetState(),
	}), nil
}

// Restore implements raft.FSM interface
func (fsm *FSM) Restore(snapshot io.ReadCloser) error {
	b, err := io.ReadAll(snapshot)

	if err != nil {
		log.Fatal(err)
		return err
	}

	data := internal.SnapshotObject{
		State:                      make(map[int]map[string]internal.KeyData),
		LatestSnapshotMilliseconds: 0,
	}

	if err = json.Unmarshal(b, &data); err != nil {
		log.Fatal(err)
		return err
	}

	// Set state
	for database, data := range internal.FilterExpiredKeys(time.Now(), data.State) {
		ctx := context.WithValue(context.Background(), "Database", database)
		for key, keyData := range data {
			if err = fsm.options.SetValues(ctx, map[string]interface{}{key: keyData.Value}); err != nil {
				log.Fatal(err)
			}
			fsm.options.SetExpiry(ctx, key, keyData.ExpireAt, false)
		}
	}

	// Set latest snapshot milliseconds.
	fsm.options.SetLatestSnapshotTime(data.LatestSnapshotMilliseconds)

	return nil
}
