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

package log_test

import (
	"bytes"
	"fmt"
	"github.com/echovault/echovault/internal/aof/log"
	"github.com/echovault/echovault/internal/clock"
	"os"
	"path"
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

func Test_AppendStore(t *testing.T) {
	t.Cleanup(func() {
		_ = os.RemoveAll(path.Join(".", "testdata"))
	})

	tests := []struct {
		name             string
		directory        string
		strategy         string
		commands         [][]string
		appendReadWriter log.ReadWriter
	}{
		{
			name:      "1. Not passing an AppendReadWriter to NewAppendStore should create a new append file",
			directory: "./testdata/log/with_no_read_writer",
			strategy:  "always",
			commands: [][]string{
				{"SET", "key1", "value1"},
				{"SET", "key2", "value2"},
				{"SET", "key3", "value3"},
			},
			appendReadWriter: nil,
		},
		{
			name:      "2. Passing an existing AppendReadWriter to NewAppendStore should successfully append and restore",
			directory: "./testdata/log/with_read_writer",
			strategy:  "always",
			commands: [][]string{
				{"SET", "key1", "value1"},
				{"SET", "key2", "value2"},
				{"SET", "key3", "value3"},
			},
			appendReadWriter: func() log.ReadWriter {
				// Create the directory if it does not exist
				if err := os.MkdirAll(path.Join("./testdata/with_read_writer", "aof"), os.ModePerm); err != nil {
					t.Error(err)
				}
				f, err := os.OpenFile(path.Join("./testdata/with_read_writer", "aof", "log.aof"),
					os.O_RDWR|os.O_CREATE|os.O_APPEND, os.ModePerm)
				if err != nil {
					t.Error(err)
				}
				return f
			}(),
		},
		{
			name:      "3. Using everysec strategy should sync the AOF file after one second",
			directory: "./testdata/log/with_everysec_strategy",
			strategy:  "everysec",
			commands: [][]string{
				{"SET", "key1", "value1"},
				{"SET", "key2", "value2"},
				{"SET", "key3", "value3"},
			},
			appendReadWriter: nil,
		},
	}

	for _, test := range tests {
		done := make(chan struct{}, 1)

		options := []func(store *log.Store){
			log.WithClock(clock.NewClock()),
			log.WithDirectory(test.directory),
			log.WithStrategy(test.strategy),
			log.WithHandleCommandFunc(func(database int, command []byte) {
				for _, c := range test.commands {
					if bytes.Contains(command, marshalRespCommand(c)) {
						return
					}
				}
				t.Errorf("could not find command in commands list:\n%s", string(command))
			}),
		}
		if test.appendReadWriter != nil {
			options = append(options, log.WithReadWriter(test.appendReadWriter))
		}

		go func() {
			store, err := log.NewAppendStore(options...)
			if err != nil {
				t.Error(err)
			}

			for _, command := range test.commands {
				b := marshalRespCommand(command)
				if err = store.Write(0, b); err != nil {
					t.Error(err)
				}
			}

			// Restore from AOF file
			if err = store.Restore(); err != nil {
				t.Error(err)
			}

			if err = store.Close(); err != nil {
				t.Error(err)
			}

			done <- struct{}{}
		}()

		ticker := time.NewTicker(200 * time.Millisecond)

		select {
		case <-done:
		case <-ticker.C:
			t.Error("timeout error")
		}
	}

}
