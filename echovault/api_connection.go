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

package echovault

import (
	"errors"
	"slices"
)

// SetProtocol sets the RESP protocol that's expected from responses to embedded API calls.
// This command does not affect the RESP protocol expected by any of the TCP clients.
//
// Parameters:
//
// `protocol` - int - The RESP version (either 2 or 3).
//
// Errors:
//
// "protocol must be either 2 or 3" - When the provided protocol is not either 2 or 3.
func (server *EchoVault) SetProtocol(protocol int) error {
	if !slices.Contains([]int{2, 3}, protocol) {
		return errors.New("protocol must be either 2 or 3")
	}
	server.connInfo.mut.Lock()
	defer server.connInfo.mut.Unlock()
	server.connInfo.embedded.Protocol = protocol
	return nil
}

// SelectDB sets the logical database to use for all embedded API calls.
// All subsequent calls after this call will use the new logical database.
// This does not affect the databases used by any of the TCP clients.
//
// Parameters:
//
// `database` - int - The Database index.
//
// Errors:
//
// "database index must be 0 or higher" - When the database index is less than 0.
func (server *EchoVault) SelectDB(database int) error {
	if database < 0 {
		return errors.New("database index must be 0 or higher")
	}
	// If the database index does not exist, create the new database.
	server.storeLock.Lock()
	if server.store[database] == nil {
		server.createDatabase(database)
	}
	server.storeLock.Unlock()

	// Set the DB.
	server.connInfo.mut.Lock()
	defer server.connInfo.mut.Unlock()
	server.connInfo.embedded.Database = database

	return nil
}
