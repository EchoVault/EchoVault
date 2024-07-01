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

// Package aof handles AOF logging in standalone mode only.
// Logging in replication clusters is handled in the raft layer.
package aof

import (
	"fmt"
	"github.com/echovault/echovault/internal"
	logstore "github.com/echovault/echovault/internal/aof/log"
	"github.com/echovault/echovault/internal/aof/preamble"
	"github.com/echovault/echovault/internal/clock"
	"log"
	"sync"
)

type Engine struct {
	clock        clock.Clock
	syncStrategy string
	directory    string
	preambleRW   preamble.ReadWriter
	appendRW     logstore.ReadWriter

	mut           sync.Mutex
	logCount      uint64
	preambleStore *preamble.Store
	appendStore   *logstore.Store

	startRewriteFunc  func()
	finishRewriteFunc func()
	getStateFunc      func() map[int]map[string]internal.KeyData
	setKeyDataFunc    func(database int, key string, data internal.KeyData)
	handleCommand     func(database int, command []byte)
}

func WithClock(clock clock.Clock) func(engine *Engine) {
	return func(engine *Engine) {
		engine.clock = clock
	}
}

func WithStrategy(strategy string) func(engine *Engine) {
	return func(engine *Engine) {
		engine.syncStrategy = strategy
	}
}

func WithDirectory(directory string) func(engine *Engine) {
	return func(engine *Engine) {
		engine.directory = directory
	}
}

func WithStartRewriteFunc(f func()) func(engine *Engine) {
	return func(engine *Engine) {
		engine.startRewriteFunc = f
	}
}

func WithFinishRewriteFunc(f func()) func(engine *Engine) {
	return func(engine *Engine) {
		engine.finishRewriteFunc = f
	}
}

func WithGetStateFunc(f func() map[int]map[string]internal.KeyData) func(engine *Engine) {
	return func(engine *Engine) {
		engine.getStateFunc = f
	}
}

func WithSetKeyDataFunc(f func(database int, key string, data internal.KeyData)) func(engine *Engine) {
	return func(engine *Engine) {
		engine.setKeyDataFunc = f
	}
}

func WithHandleCommandFunc(f func(database int, command []byte)) func(engine *Engine) {
	return func(engine *Engine) {
		engine.handleCommand = f
	}
}

func WithPreambleReadWriter(rw preamble.ReadWriter) func(engine *Engine) {
	return func(engine *Engine) {
		engine.preambleRW = rw
	}
}

func WithAppendReadWriter(rw logstore.ReadWriter) func(engine *Engine) {
	return func(engine *Engine) {
		engine.appendRW = rw
	}
}

func NewAOFEngine(options ...func(engine *Engine)) (*Engine, error) {
	engine := &Engine{
		clock:             clock.NewClock(),
		syncStrategy:      "everysec",
		directory:         "",
		mut:               sync.Mutex{},
		logCount:          0,
		startRewriteFunc:  func() {},
		finishRewriteFunc: func() {},
		getStateFunc:      func() map[int]map[string]internal.KeyData { return nil },
		setKeyDataFunc:    func(database int, key string, data internal.KeyData) {},
		handleCommand:     func(database int, command []byte) {},
	}

	// Setup AOFEngine options first as these options are used
	// when setting up the PreambleStore and AppendStore
	for _, option := range options {
		option(engine)
	}

	// Setup Preamble engine
	preambleStore, err := preamble.NewPreambleStore(
		preamble.WithClock(engine.clock),
		preamble.WithDirectory(engine.directory),
		preamble.WithReadWriter(engine.preambleRW),
		preamble.WithGetStateFunc(engine.getStateFunc),
		preamble.WithSetKeyDataFunc(engine.setKeyDataFunc),
	)
	if err != nil {
		return nil, err
	}
	engine.preambleStore = preambleStore

	// Setup AOF log store engine
	appendStore, err := logstore.NewAppendStore(
		logstore.WithClock(engine.clock),
		logstore.WithDirectory(engine.directory),
		logstore.WithStrategy(engine.syncStrategy),
		logstore.WithReadWriter(engine.appendRW),
		logstore.WithHandleCommandFunc(engine.handleCommand),
	)
	if err != nil {
		return nil, err
	}
	engine.appendStore = appendStore

	return engine, nil
}

func (engine *Engine) LogCommand(database int, command []byte) {
	if err := engine.appendStore.Write(database, command); err != nil {
		log.Printf("log command error: %+v\n", err)
	}
}

func (engine *Engine) RewriteLog() error {
	engine.mut.Lock()
	defer engine.mut.Unlock()

	engine.startRewriteFunc()
	defer engine.finishRewriteFunc()

	// Create AOF preamble.
	if err := engine.preambleStore.CreatePreamble(); err != nil {
		return fmt.Errorf("rewrite log error: create preamble error: %+v", err)
	}

	// Truncate the AOF file.
	if err := engine.appendStore.Truncate(); err != nil {
		return fmt.Errorf("rewrite log error: create aof error: %+v", err)
	}

	return nil
}

func (engine *Engine) Restore() error {
	if err := engine.preambleStore.Restore(); err != nil {
		return fmt.Errorf("restore aof error: restore preamble error: %+v", err)
	}
	if err := engine.appendStore.Restore(); err != nil {
		return fmt.Errorf("restore aof error: restore aof error: %+v", err)
	}
	return nil
}
