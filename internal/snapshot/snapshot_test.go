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

package snapshot_test

import (
	"fmt"
	"github.com/echovault/echovault/internal"
	"github.com/echovault/echovault/internal/clock"
	"github.com/echovault/echovault/internal/snapshot"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func Test_SnapshotEngine(t *testing.T) {
	mockClock := clock.NewClock()
	directory := "./testdata"
	var threshold uint64 = 5

	var snapshotInProgress atomic.Bool
	startSnapshotFunc := func() {
		if snapshotInProgress.Load() {
			t.Error("expected snapshotInProgress to be false, got true")
		}
		snapshotInProgress.Store(true)
	}
	finishSnapshotFunc := func() {
		if !snapshotInProgress.Load() {
			t.Error("expected snapshotInProgress to be true, got false")
		}
		snapshotInProgress.Store(false)
	}

	state := map[int]map[string]internal.KeyData{
		0: {
			"key1": {Value: "value-01", ExpireAt: clock.NewClock().Now().Add(13 * time.Second)},
			"key2": {Value: "value-02", ExpireAt: clock.NewClock().Now().Add(43 * time.Minute)},
			"key3": {Value: "value-03", ExpireAt: clock.NewClock().Now().Add(112 * time.Millisecond)},
			"key4": {Value: "value-04", ExpireAt: clock.NewClock().Now().Add(23 * time.Second)},
			"key5": {Value: "value-45", ExpireAt: clock.NewClock().Now().Add(121 * time.Millisecond)},
		},
		1: {
			"key1": {Value: "value1", ExpireAt: clock.NewClock().Now().Add(13 * time.Second)},
			"key2": {Value: "value2", ExpireAt: clock.NewClock().Now().Add(43 * time.Minute)},
			"key3": {Value: "value3", ExpireAt: clock.NewClock().Now().Add(112 * time.Millisecond)},
			"key4": {Value: "value4", ExpireAt: clock.NewClock().Now().Add(23 * time.Second)},
			"key5": {Value: "value5", ExpireAt: clock.NewClock().Now().Add(121 * time.Millisecond)},
		},
	}

	getStateFunc := func() map[int]map[string]internal.KeyData {
		return state
	}

	restoredState := make(map[int]map[string]internal.KeyData)
	setKeyDataFunc := func(database int, key string, data internal.KeyData) {
		if restoredState[database] == nil {
			restoredState[database] = make(map[string]internal.KeyData)
		}
		restoredState[database][key] = data
	}

	var latestSnapshotTime int64
	setLatestSnapshotTimeFunc := func(msec int64) {
		latestSnapshotTime = msec
	}
	getLatestSnapshotTimeFunc := func() int64 {
		return latestSnapshotTime
	}

	snapshotEngine := snapshot.NewSnapshotEngine(
		snapshot.WithClock(mockClock),
		snapshot.WithDirectory(directory),
		snapshot.WithInterval(10*time.Millisecond),
		snapshot.WithThreshold(threshold),
		snapshot.WithStartSnapshotFunc(startSnapshotFunc),
		snapshot.WithFinishSnapshotFunc(finishSnapshotFunc),
		snapshot.WithGetStateFunc(getStateFunc),
		snapshot.WithSetKeyDataFunc(setKeyDataFunc),
		snapshot.WithSetLatestSnapshotTimeFunc(setLatestSnapshotTimeFunc),
		snapshot.WithGetLatestSnapshotTimeFunc(getLatestSnapshotTimeFunc),
	)

	if err := snapshotEngine.TakeSnapshot(); err != nil {
		t.Error(err)
	}

	// Add more records to each database in the state
	for database, _ := range state {
		for i := 0; i < 5; i++ {
			state[database][fmt.Sprintf("key%d", i)] = internal.KeyData{
				Value:    fmt.Sprintf("value%d", i),
				ExpireAt: clock.NewClock().Now().Add(time.Duration(i) * time.Second),
			}
		}
	}

	// Take another snapshot
	if err := snapshotEngine.TakeSnapshot(); err != nil {
		t.Error(err)
	}

	if err := snapshotEngine.Restore(); err != nil {
		t.Error(err)
	}

	if len(restoredState) != len(state) {
		t.Errorf("expected restored state to be length %d, got %d", len(state), len(restoredState))
	}

	for database, data := range restoredState {
		for key, keyData := range data {
			if state[database][key].Value != keyData.Value {
				t.Errorf("expected value %v for key %s, got %v", state[database][key].Value, key, keyData.Value)
			}
			if !state[database][key].ExpireAt.Equal(keyData.ExpireAt) {
				t.Errorf("expected expiry time %v for key %s, got %v", state[database][key].ExpireAt, key, keyData.ExpireAt)
			}
		}
	}

	_ = os.RemoveAll(directory)
}
