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

package aof_test

import (
	"fmt"
	"github.com/echovault/echovault/internal"
	"github.com/echovault/echovault/internal/aof"
	"github.com/echovault/echovault/internal/aof/log"
	"github.com/echovault/echovault/internal/aof/preamble"
	"github.com/echovault/echovault/internal/clock"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func marshalRespCommand(command []string) []byte {
	return []byte(fmt.Sprintf(
		"*%d\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n", len(command),
		len(command[0]), command[0],
		len(command[1]), command[1],
		len(command[2]), command[2],
	))
}

func Test_AOFEngine(t *testing.T) {
	strategy := "always"
	directory := "./testdata"

	var rewriteInProgress atomic.Bool
	startRewriteFunc := func() {
		if rewriteInProgress.Load() {
			t.Error("expected rewriteInProgress to be false, got true")
		}
		rewriteInProgress.Store(true)
	}
	finishRewriteFunc := func() {
		if !rewriteInProgress.Load() {
			t.Error("expected rewriteInProgress to be true, got false")
			rewriteInProgress.Store(false)
		}
	}

	state := map[int]map[string]internal.KeyData{
		0: {
			"key1": {Value: "value-01", ExpireAt: clock.NewClock().Now().Add(10 * time.Second)},
			"key2": {Value: "value-02", ExpireAt: clock.NewClock().Now().Add(-10 * time.Second)}, // Should be excluded on restore
			"key3": {Value: "value-03", ExpireAt: clock.NewClock().Now().Add(10 * time.Second)},
		},
		1: {
			"key1": {Value: "value-11", ExpireAt: clock.NewClock().Now().Add(10 * time.Second)},
			"key2": {Value: "value-12", ExpireAt: clock.NewClock().Now().Add(-10 * time.Second)}, // Should be excluded on restore
			"key3": {Value: "value-13", ExpireAt: clock.NewClock().Now().Add(-10 * time.Second)}, // Should be excluded on restore
			"key4": {Value: "value-14", ExpireAt: clock.NewClock().Now().Add(10 * time.Second)},
		},
	}
	restoredState := map[int]map[string]internal.KeyData{}
	wantRestoredState := map[int]map[string]internal.KeyData{
		0: {
			"key1":  {Value: "value-01", ExpireAt: clock.NewClock().Now().Add(10 * time.Second)},
			"key3":  {Value: "value-03", ExpireAt: clock.NewClock().Now().Add(10 * time.Second)},
			"key4":  {Value: "value-04", ExpireAt: time.Time{}},
			"key5":  {Value: "value-05", ExpireAt: time.Time{}},
			"key6":  {Value: "value-06", ExpireAt: time.Time{}},
			"key7":  {Value: "value-07", ExpireAt: time.Time{}},
			"key8":  {Value: "value-08", ExpireAt: time.Time{}},
			"key9":  {Value: "value-09", ExpireAt: time.Time{}},
			"key10": {Value: "value-010", ExpireAt: time.Time{}},
		},
		1: {
			"key1":  {Value: "value-11", ExpireAt: clock.NewClock().Now().Add(10 * time.Second)},
			"key4":  {Value: "value-14", ExpireAt: time.Time{}},
			"key5":  {Value: "value-15", ExpireAt: time.Time{}},
			"key6":  {Value: "value-16", ExpireAt: time.Time{}},
			"key7":  {Value: "value-17", ExpireAt: time.Time{}},
			"key8":  {Value: "value-18", ExpireAt: time.Time{}},
			"key9":  {Value: "value-19", ExpireAt: time.Time{}},
			"key10": {Value: "value-110", ExpireAt: time.Time{}},
		},
	}

	getStateFunc := func() map[int]map[string]internal.KeyData {
		return state
	}

	setKeyDataFunc := func(database int, key string, data internal.KeyData) {
		if restoredState[database] == nil {
			restoredState[database] = make(map[string]internal.KeyData)
		}
		restoredState[database][key] = data
	}

	handleCommandFunc := func(database int, command []byte) {
		cmd, err := internal.Decode(command)
		if err != nil {
			t.Error(err)
		}
		restoredState[database][cmd[1]] = internal.KeyData{Value: cmd[2], ExpireAt: time.Time{}}
	}

	preambleReadWriter := func() preamble.ReadWriter {
		return nil
	}()
	appendReadWriter := func() log.ReadWriter {
		return nil
	}()

	engine, err := aof.NewAOFEngine(
		aof.WithClock(clock.NewClock()),
		aof.WithStrategy(strategy),
		aof.WithDirectory(directory),
		aof.WithStartRewriteFunc(startRewriteFunc),
		aof.WithFinishRewriteFunc(finishRewriteFunc),
		aof.WithGetStateFunc(getStateFunc),
		aof.WithSetKeyDataFunc(setKeyDataFunc),
		aof.WithHandleCommandFunc(handleCommandFunc),
		aof.WithPreambleReadWriter(preambleReadWriter),
		aof.WithAppendReadWriter(appendReadWriter),
	)
	if err != nil {
		t.Error(err)
	}

	// Log some commands to mutate the state
	preRewriteCommands := map[int][][]string{
		0: {
			{"SET", "key4", "value4"},
			{"SET", "key5", "value5"},
			{"SET", "key6", "value6"},
		},
		1: {
			{"SET", "key4", "value4"},
			{"SET", "key5", "value5"},
			{"SET", "key6", "value6"},
		},
	}

	for database, commands := range preRewriteCommands {
		for _, command := range commands {
			state[database][command[1]] = internal.KeyData{Value: command[2], ExpireAt: time.Time{}}
			engine.LogCommand(database, marshalRespCommand(command))
		}
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer func() {
		ticker.Stop()
	}()

	<-ticker.C

	// Trigger log rewrite
	if err = engine.RewriteLog(); err != nil {
		t.Error(err)
	}

	// Log some more commands
	postRewriteCommands := map[int][][]string{
		0: {
			{"SET", "key7", "value7"},
			{"SET", "key8", "value8"},
			{"SET", "key9", "value9"},
			{"SET", "key10", "value10"},
		},
		1: {
			{"SET", "key7", "value7"},
			{"SET", "key8", "value8"},
			{"SET", "key9", "value9"},
			{"SET", "key10", "value10"},
		},
	}

	for database, commands := range postRewriteCommands {
		for _, command := range commands {
			state[database][command[1]] = internal.KeyData{Value: command[2], ExpireAt: time.Time{}}
			engine.LogCommand(database, marshalRespCommand(command))
		}
	}

	ticker.Reset(100 * time.Millisecond)
	<-ticker.C

	// Restore logs
	if err = engine.Restore(); err != nil {
		t.Error(err)
	}

	if len(wantRestoredState) != len(restoredState) {
		t.Errorf("expected restored state to be length %d, got %d", len(wantRestoredState), len(restoredState))
		for database, data := range restoredState {
			for key, keyData := range data {
				want, ok := wantRestoredState[database][key]
				if !ok {
					t.Errorf("could not find key %s in expected state", key)
				}
				if want.Value != keyData.Value {
					t.Errorf("expected value %v for key %s, got %v", want.Value, key, keyData.Value)
				}
				if !want.ExpireAt.Equal(keyData.ExpireAt) {
					t.Errorf("expected expiry time of %v for key %s, got %v", want.ExpireAt, key, keyData.ExpireAt)
				}
			}
		}
	}

	_ = os.RemoveAll(directory)
}
