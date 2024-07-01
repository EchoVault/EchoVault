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

package snapshot

import (
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/echovault/echovault/internal"
	"github.com/echovault/echovault/internal/clock"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"sync/atomic"
	"time"
)

// This package contains the snapshot engine for standalone mode.
// Snapshots in cluster mode will be handled using the raft package in the raft layer.

type Manifest struct {
	LatestSnapshotMilliseconds int64
	LatestSnapshotHash         [16]byte
}

type Engine struct {
	clock                     clock.Clock
	changeCount               atomic.Uint64
	directory                 string
	snapshotInterval          time.Duration
	snapshotThreshold         uint64
	startSnapshotFunc         func()
	finishSnapshotFunc        func()
	getStateFunc              func() map[int]map[string]internal.KeyData
	setLatestSnapshotTimeFunc func(msec int64)
	getLatestSnapshotTimeFunc func() int64
	setKeyDataFunc            func(database int, key string, data internal.KeyData)
}

func WithClock(clock clock.Clock) func(engine *Engine) {
	return func(engine *Engine) {
		engine.clock = clock
	}
}

func WithDirectory(directory string) func(engine *Engine) {
	return func(engine *Engine) {
		engine.directory = directory
	}
}

func WithInterval(interval time.Duration) func(engine *Engine) {
	return func(engine *Engine) {
		engine.snapshotInterval = interval
	}
}

func WithThreshold(threshold uint64) func(engine *Engine) {
	return func(engine *Engine) {
		engine.snapshotThreshold = threshold
	}
}

func WithStartSnapshotFunc(f func()) func(engine *Engine) {
	return func(engine *Engine) {
		engine.startSnapshotFunc = f
	}
}

func WithFinishSnapshotFunc(f func()) func(engine *Engine) {
	return func(engine *Engine) {
		engine.finishSnapshotFunc = f
	}
}

func WithGetStateFunc(f func() map[int]map[string]internal.KeyData) func(engine *Engine) {
	return func(engine *Engine) {
		engine.getStateFunc = f
	}
}

func WithSetLatestSnapshotTimeFunc(f func(mset int64)) func(engine *Engine) {
	return func(engine *Engine) {
		engine.setLatestSnapshotTimeFunc = f
	}
}

func WithGetLatestSnapshotTimeFunc(f func() int64) func(engine *Engine) {
	return func(engine *Engine) {
		engine.getLatestSnapshotTimeFunc = f
	}
}

func WithSetKeyDataFunc(f func(database int, key string, data internal.KeyData)) func(engine *Engine) {
	return func(engine *Engine) {
		engine.setKeyDataFunc = f
	}
}

func NewSnapshotEngine(options ...func(engine *Engine)) *Engine {
	engine := &Engine{
		clock:              clock.NewClock(),
		changeCount:        atomic.Uint64{},
		directory:          "",
		snapshotInterval:   5 * time.Minute,
		snapshotThreshold:  1000,
		startSnapshotFunc:  func() {},
		finishSnapshotFunc: func() {},
		getStateFunc: func() map[int]map[string]internal.KeyData {
			return make(map[int]map[string]internal.KeyData)
		},
		setKeyDataFunc:            func(database int, key string, data internal.KeyData) {},
		setLatestSnapshotTimeFunc: func(msec int64) {},
		getLatestSnapshotTimeFunc: func() int64 {
			return 0
		},
	}

	for _, option := range options {
		option(engine)
	}

	if engine.snapshotInterval != 0 {
		go func() {
			ticker := time.NewTicker(engine.snapshotInterval)
			defer func() {
				ticker.Stop()
			}()
			for {
				<-ticker.C
				if engine.changeCount.Load() == engine.snapshotThreshold {
					if err := engine.TakeSnapshot(); err != nil {
						log.Println(err)
					}
				}
			}
		}()
	}

	return engine
}

func (engine *Engine) TakeSnapshot() error {
	engine.startSnapshotFunc()
	defer engine.finishSnapshotFunc()

	// Extract current time
	msec := engine.clock.Now().UnixMilli()

	// Update manifest file to indicate the latest snapshot.
	// If manifest file does not exist, create it.
	// Manifest object will contain the following information:
	// 	1. Hash of the snapshot contents.
	// 	2. Unix time of the latest snapshot taken.
	// The information above will be used to determine whether a snapshot should be taken.
	// If the hash of the current state equals the hash in the manifest file, skip the snapshot.
	// Otherwise, take the snapshot and update the latest snapshot timestamp and hash in the manifest file.

	var firstSnapshot bool // Tracks whether the snapshot being attempted is the first one

	dirname := path.Join(engine.directory, "snapshots")
	if err := os.MkdirAll(dirname, os.ModePerm); err != nil {
		log.Println(err)
		return err
	}

	// Open manifest file
	var mf *os.File
	mf, err := os.Open(path.Join(dirname, "manifest.bin"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Create file if it does not exist
			mf, err = os.Create(path.Join(dirname, "manifest.bin"))
			if err != nil {
				log.Println(err)
				return err
			}
			firstSnapshot = true
		} else {
			log.Println(err)
			return err
		}
	}

	md, err := io.ReadAll(mf)
	if err != nil {
		log.Println(err)
		return err
	}
	if err := mf.Close(); err != nil {
		log.Println(err)
		return err
	}

	manifest := new(Manifest)

	if !firstSnapshot {
		if err = json.Unmarshal(md, manifest); err != nil {
			log.Println(err)
			return err
		}
	}

	// Get current state
	snapshotObject := internal.SnapshotObject{
		State:                      internal.FilterExpiredKeys(engine.clock.Now(), engine.getStateFunc()),
		LatestSnapshotMilliseconds: engine.getLatestSnapshotTimeFunc(),
	}
	out, err := json.Marshal(snapshotObject)
	if err != nil {
		log.Println(err)
		return err
	}

	snapshotHash := md5.Sum(out)
	if snapshotHash == manifest.LatestSnapshotHash {
		return errors.New("nothing new to snapshot")
	}

	// Update the snapshotObject
	snapshotObject.LatestSnapshotMilliseconds = msec
	// Marshal the updated snapshotObject
	out, err = json.Marshal(snapshotObject)
	if err != nil {
		log.Println(err)
		return err
	}

	// os.Create will replace the old manifest file
	mf, err = os.Create(path.Join(dirname, "manifest.bin"))
	if err != nil {
		log.Println(err)
		return err
	}

	// Write the latest manifest data
	manifest = &Manifest{
		LatestSnapshotHash:         md5.Sum(out),
		LatestSnapshotMilliseconds: msec,
	}
	mo, err := json.Marshal(manifest)
	if err != nil {
		log.Println(err)
		return err
	}
	if _, err = mf.Write(mo); err != nil {
		log.Println(err)
		return err
	}
	if err = mf.Sync(); err != nil {
		log.Println(err)
	}
	if err = mf.Close(); err != nil {
		log.Println(err)
		return err
	}

	// Create snapshot directory
	dirname = path.Join(engine.directory, "snapshots", fmt.Sprintf("%d", msec))
	if err := os.MkdirAll(dirname, os.ModePerm); err != nil {
		return err
	}

	// Create snapshot file
	f, err := os.OpenFile(path.Join(dirname, "state.bin"), os.O_WRONLY|os.O_CREATE, os.ModePerm)
	if err != nil {
		log.Println(err)
		return err
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Println(err)
		}
	}()

	// Write state to file
	if _, err = f.Write(out); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		log.Println(err)
	}

	// Set the latest snapshot in unix milliseconds
	engine.setLatestSnapshotTimeFunc(msec)

	// Reset the change count
	engine.resetChangeCount()

	return nil
}

func (engine *Engine) Restore() error {
	mf, err := os.Open(path.Join(engine.directory, "snapshots", "manifest.bin"))
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		return errors.New("no snapshot manifest, skipping snapshot restore")
	}
	if err != nil {
		return err
	}

	manifest := new(Manifest)

	md, err := io.ReadAll(mf)
	if err != nil {
		return err
	}

	if err = json.Unmarshal(md, manifest); err != nil {
		return err
	}

	if manifest.LatestSnapshotMilliseconds == 0 {
		return errors.New("no snapshot to restore")
	}

	sf, err := os.Open(path.Join(
		engine.directory,
		"snapshots",
		fmt.Sprintf("%d", manifest.LatestSnapshotMilliseconds),
		"state.bin"))
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("snapshot file %d/state.bin not found, skipping snapshot", manifest.LatestSnapshotMilliseconds)
	}
	if err != nil {
		return err
	}

	sd, err := io.ReadAll(sf)
	if err != nil {
		return nil
	}

	snapshotObject := new(internal.SnapshotObject)
	if err = json.Unmarshal(sd, snapshotObject); err != nil {
		return err
	}

	engine.setLatestSnapshotTimeFunc(snapshotObject.LatestSnapshotMilliseconds)

	for database, data := range internal.FilterExpiredKeys(engine.clock.Now(), snapshotObject.State) {
		for key, keyData := range data {
			engine.setKeyDataFunc(database, key, keyData)
		}
	}

	log.Println("successfully restored latest snapshot")

	return nil
}

func (engine *Engine) IncrementChangeCount() {
	engine.changeCount.Add(1)
}

func (engine *Engine) resetChangeCount() {
	engine.changeCount.Store(0)
}
