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

package log

import (
	"fmt"
	"github.com/echovault/echovault/internal"
	"github.com/echovault/echovault/internal/clock"
	"github.com/tidwall/resp"
	"io"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ReadWriter interface {
	io.ReadWriteSeeker
	io.Closer
	Truncate(size int64) error
	Sync() error
}

type Store struct {
	clock clock.Clock
	// Keeps track of the current database that we're logging commands for.
	currentDatabase int
	// Append file sync strategy. Can only be "always", "everysec", or "no".
	strategy string
	// Store mutex.
	mut sync.Mutex
	// The ReadWriter used to persist and load the log.
	rw ReadWriter
	// The directory for the AOF file if we must create one.
	directory string
	// Function to handle command read from AOF log after restore.
	handleCommand func(database int, command []byte)
}

func WithClock(clock clock.Clock) func(store *Store) {
	return func(store *Store) {
		store.clock = clock
	}
}

func WithStrategy(strategy string) func(store *Store) {
	return func(store *Store) {
		store.strategy = strings.ToLower(strategy)
	}
}

func WithReadWriter(rw ReadWriter) func(store *Store) {
	return func(store *Store) {
		store.rw = rw
	}
}

func WithDirectory(directory string) func(store *Store) {
	return func(store *Store) {
		store.directory = directory
	}
}

func WithHandleCommandFunc(f func(database int, command []byte)) func(store *Store) {
	return func(store *Store) {
		store.handleCommand = f
	}
}

func NewAppendStore(options ...func(store *Store)) (*Store, error) {
	store := &Store{
		clock:           clock.NewClock(),
		currentDatabase: -1,
		directory:       "",
		strategy:        "everysec",
		rw:              nil,
		mut:             sync.Mutex{},
		handleCommand:   func(database int, command []byte) {},
	}

	for _, option := range options {
		option(store)
	}

	// If rw is nil, use a default file at the provided directory
	if store.rw == nil && store.directory != "" {
		// Create the directory if it does not exist
		err := os.MkdirAll(path.Join(store.directory, "aof"), os.ModePerm)
		if err != nil {
			return nil, fmt.Errorf("new append store -> mkdir error: %+v", err)
		}
		f, err := os.OpenFile(path.Join(store.directory, "aof", "log.aof"), os.O_RDWR|os.O_CREATE|os.O_APPEND, os.ModePerm)
		if err != nil {
			return nil, fmt.Errorf("new append store -> open file error: %+v", err)
		}
		store.rw = f
	}

	// Start another goroutine that takes handles syncing the content to the file system.
	// No need to start this goroutine if sync strategy is anything other than 'everysec'.
	if strings.EqualFold(store.strategy, "everysec") {
		go func() {
			ticker := time.NewTicker(1 * time.Second)
			defer func() {
				ticker.Stop()
			}()
			for {
				store.mut.Lock()
				if err := store.Sync(); err != nil {
					store.mut.Unlock()
					log.Println(fmt.Errorf("new append store error: %+v", err))
					break
				}
				store.mut.Unlock()
				<-ticker.C
			}
		}()
	}

	return store, nil
}

func (store *Store) Write(database int, command []byte) error {
	// Skip operation if ReadWriter is not defined.
	if store.rw == nil {
		return nil
	}

	store.mut.Lock()
	defer store.mut.Unlock()

	// If the database parameter is different from the current database index,
	// log the SELECT command before logging the incoming command.
	// This allows us to switch databases appropriately when restoring the state on startup.
	if database != store.currentDatabase {
		_, err := store.rw.Write([]byte(fmt.Sprintf("*2\r\n$6\r\nSELECT\r\n$1\r\n%s\r\n", strconv.Itoa(database))))
		if err != nil {
			return fmt.Errorf("log select error: %+v", err)
		}
		store.currentDatabase = database
	}

	if _, err := store.rw.Write(command); err != nil {
		return fmt.Errorf("log command error: %+v", err)
	}

	if strings.EqualFold(store.strategy, "always") {
		if err := store.Sync(); err != nil {
			return fmt.Errorf("log file sync error: %+v", err)
		}
	}

	return nil
}

func (store *Store) Sync() error {
	if store.rw != nil {
		return store.rw.Sync()
	}
	return nil
}

func (store *Store) Restore() error {
	store.mut.Lock()
	defer store.mut.Unlock()

	// Move cursor to the beginning of the file
	if _, err := store.rw.Seek(0, 0); err != nil {
		return fmt.Errorf("restore aof: %v", err)
	}

	r := resp.NewReader(store.rw)
	database := 0

	for {
		value, n, err := r.ReadValue()
		if err != nil && err != io.EOF {
			return err
		}
		if n == 0 {
			// Break out when there are no more bytes to read.
			break
		}

		command, err := value.MarshalRESP()
		if err != nil {
			return err
		}

		// Decode command.
		cmd, err := internal.Decode(command)
		if err != nil {
			return err
		}
		// If the command is a SELECT command, set the database value.
		if strings.EqualFold(cmd[0], "select") {
			database, err = strconv.Atoi(cmd[1])
			if err != nil {
				return err
			}
			// Restart the read loop.
			continue
		}

		store.handleCommand(database, command)
	}

	return nil
}

func (store *Store) Truncate() error {
	store.mut.Lock()
	defer store.mut.Unlock()

	if err := store.rw.Truncate(0); err != nil {
		return fmt.Errorf("truncate: truncate error: %+v", err)
	}

	// Seek to the beginning of the file after truncating.
	if _, err := store.rw.Seek(0, 0); err != nil {
		return fmt.Errorf("truncate: seek error: %+v", err)
	}

	// Add command to select the current database at the top of the file.
	_, err := store.rw.Write([]byte(
		fmt.Sprintf("*2\r\n$6\r\nSELECT\r\n$1\r\n%s\r\n", strconv.Itoa(store.currentDatabase))))
	if err != nil {
		return fmt.Errorf("truncate: log select error: %+v", err)
	}
	// Immediately sync the file.
	if err = store.rw.Sync(); err != nil {
		return fmt.Errorf("truncate: sync error: %+v", err)
	}

	return nil
}

func (store *Store) Close() error {
	store.mut.Lock()
	defer store.mut.Unlock()
	return store.rw.Close()
}
