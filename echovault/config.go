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
	"github.com/echovault/echovault/internal"
	"github.com/echovault/echovault/internal/config"
	"github.com/echovault/echovault/internal/constants"
)

// DefaultConfig returns the default configuration.
// This should be used when using EchoVault as an embedded library.
func DefaultConfig() config.Config {
	return config.DefaultConfig()
}

func (server *EchoVault) GetServerInfo() internal.ServerInfo {
	return internal.ServerInfo{
		Server:  "echovault",
		Version: constants.Version,
		Id:      server.config.ServerID,
		Mode: func() string {
			if server.isInCluster() {
				return "cluster"
			}
			return "standalone"
		}(),
		Role: func() string {
			if !server.isInCluster() {
				return "master"
			}
			if server.raft.IsRaftLeader() {
				return "master"
			}
			return "replica"
		}(),
		Modules: server.ListModules(),
	}
}
