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

package sugardb

import (
	"context"
	"time"

	"github.com/echovault/sugardb/internal"
	"github.com/echovault/sugardb/internal/config"
	"github.com/echovault/sugardb/internal/constants"
)

// DefaultConfig returns the default configuration.
// This should be used when using SugarDB as an embedded library.
func DefaultConfig() config.Config {
	return config.DefaultConfig()
}

func (server *SugarDB) GetServerInfo() internal.ServerInfo {
	return internal.ServerInfo{
		Server:  "sugardb",
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
		Modules:    server.ListModules(),
		MemoryUsed: server.memUsed,
		MaxMemory:  server.config.MaxMemory,
	}
}

// WithTLS is an option to the NewSugarDB function that allows you to pass a
// custom TLS to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithTLS(b ...bool) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		if len(b) > 0 {
			sugardb.config.TLS = b[0]
		} else {
			sugardb.config.TLS = true
		}
	}
}

// WithContext is an options that for the NewSugarDB function that allows you to
// configure a custom context object to be used in SugarDB.
// If you don't provide this option, SugarDB will create its own internal context object.
func WithContext(ctx context.Context) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.context = ctx
	}
}

// WithConfig is an option for the NewSugarDB function that allows you to pass a
// custom configuration to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithConfig(config config.Config) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config = config
	}
}

// WithMTLS is an option to the NewSugarDB function that allows you to pass a
// custom MTLS to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithMTLS(b ...bool) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		if len(b) > 0 {
			sugardb.config.MTLS = b[0]
		} else {
			sugardb.config.MTLS = true
		}
	}
}

// CertKeyPair defines the paths to the cert and key pair files respectively.
type CertKeyPair struct {
	Cert string
	Key  string
}

// WithCertKeyPairs is an option to the NewSugarDB function that allows you to pass a
// custom CertKeyPairs to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithCertKeyPairs(certKeyPairs []CertKeyPair) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		for _, pair := range certKeyPairs {
			sugardb.config.CertKeyPairs = append(sugardb.config.CertKeyPairs, []string{pair.Cert, pair.Key})
		}
	}
}

// WithClientCAs is an option to the NewSugarDB function that allows you to pass a
// custom ClientCAs to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithClientCAs(clientCAs []string) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.ClientCAs = clientCAs
	}
}

// WithPort is an option to the NewSugarDB function that allows you to pass a
// custom Port to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithPort(port uint16) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.Port = port
	}
}

// WithServerID is an option to the NewSugarDB function that allows you to pass a
// custom ServerID to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithServerID(serverID string) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.ServerID = serverID
	}
}

// WithJoinAddr is an option to the NewSugarDB function that allows you to pass a
// custom JoinAddr to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithJoinAddr(joinAddr string) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.JoinAddr = joinAddr
	}
}

// WithAdvertiseAddr is an option to the NewSugarDB function that allows you to pass a
// custom AdvertiseAddr to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithAdvertiseAddr(advertiseAddr string) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.AdvertiseAddr = advertiseAddr
	}
}

// WithBindAddr is an option to the NewSugarDB function that allows you to pass a
// custom BindAddr to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithBindAddr(bindAddr string) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.BindAddr = bindAddr
	}
}

// WithDataDir is an option to the NewSugarDB function that allows you to pass a
// custom DataDir to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithDataDir(dataDir string) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.DataDir = dataDir
	}
}

// WithBootstrapCluster is an option to the NewSugarDB function that allows you to pass a
// custom BootstrapCluster to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithBootstrapCluster(b ...bool) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		if len(b) > 0 {
			sugardb.config.BootstrapCluster = b[0]
		} else {
			sugardb.config.BootstrapCluster = true
		}
	}
}

// WithAclConfig is an option to the NewSugarDB function that allows you to pass a
// custom AclConfig to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithAclConfig(aclConfig string) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.AclConfig = aclConfig
	}
}

// WithForwardCommand is an option to the NewSugarDB function that allows you to pass a
// custom ForwardCommand to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithForwardCommand(b ...bool) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		if len(b) > 0 {
			sugardb.config.ForwardCommand = b[0]
		} else {
			sugardb.config.ForwardCommand = true
		}
	}
}

// WithRequirePass is an option to the NewSugarDB function that allows you to pass a
// custom RequirePass to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithRequirePass(b ...bool) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		if len(b) > 0 {
			sugardb.config.RequirePass = b[0]
		} else {
			sugardb.config.RequirePass = true
		}
	}
}

// WithPassword is an option to the NewSugarDB function that allows you to pass a
// custom Password to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithPassword(password string) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.Password = password
	}
}

// WithSnapShotThreshold is an option to the NewSugarDB function that allows you to pass a
// custom SnapShotThreshold to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithSnapShotThreshold(snapShotThreshold uint64) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.SnapShotThreshold = snapShotThreshold
	}
}

// WithSnapshotInterval is an option to the NewSugarDB function that allows you to pass a
// custom SnapshotInterval to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithSnapshotInterval(snapshotInterval time.Duration) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.SnapshotInterval = snapshotInterval
	}
}

// WithRestoreSnapshot is an option to the NewSugarDB function that allows you to pass a
// custom RestoreSnapshot to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithRestoreSnapshot(b ...bool) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		if len(b) > 0 {
			sugardb.config.RestoreSnapshot = b[0]
		} else {
			sugardb.config.RestoreSnapshot = true
		}
	}
}

// WithRestoreAOF is an option to the NewSugarDB function that allows you to pass a
// custom RestoreAOF to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithRestoreAOF(b ...bool) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		if len(b) > 0 {
			sugardb.config.RestoreAOF = b[0]
		} else {
			sugardb.config.RestoreAOF = true
		}
	}
}

// WithAOFSyncStrategy is an option to the NewSugarDB function that allows you to pass a
// custom AOFSyncStrategy to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithAOFSyncStrategy(aOFSyncStrategy string) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.AOFSyncStrategy = aOFSyncStrategy
	}
}

// WithMaxMemory is an option to the NewSugarDB function that allows you to pass a
// custom MaxMemory to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithMaxMemory(maxMemory uint64) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.MaxMemory = maxMemory
	}
}

// WithEvictionPolicy is an option to the NewSugarDB function that allows you to pass a
// custom EvictionPolicy to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithEvictionPolicy(evictionPolicy string) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.EvictionPolicy = evictionPolicy
	}
}

// WithEvictionSample is an option to the NewSugarDB function that allows you to pass a
// custom EvictionSample to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithEvictionSample(evictionSample uint) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.EvictionSample = evictionSample
	}
}

// WithEvictionInterval is an option to the NewSugarDB function that allows you to pass a
// custom EvictionInterval to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithEvictionInterval(evictionInterval time.Duration) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.EvictionInterval = evictionInterval
	}
}

// WithElectionTimeout is an option to the NewSugarDB function that allows you to pass a
// custom ElectionTimeout to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithElectionTimeout(electionTimeout time.Duration) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.ElectionTimeout = electionTimeout
	}
}

// WithHeartbeatTimeout is an option to the NewSugarDB function that allows you to pass a
// custom HeartbeatTimeout to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithHeartbeatTimeout(heartbeatTimeout time.Duration) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.HeartbeatTimeout = heartbeatTimeout
	}
}

// WithHeartbeatTimeout is an option to the NewSugarDB function that allows you to pass a
// custom HeartbeatTimeout to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithCommitTimeout(commitTimeout time.Duration) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.CommitTimeout = commitTimeout
	}
}

// WithModules is an option to the NewSugarDB function that allows you to pass a
// custom Modules to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithModules(modules []string) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.Modules = modules
	}
}

// WithDiscoveryPort is an option to the NewSugarDB function that allows you to pass a
// custom DiscoveryPort to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithDiscoveryPort(discoveryPort uint16) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.DiscoveryPort = discoveryPort
	}
}

// WithRaftBindAddr is an option to the NewSugarDB function that allows you to pass a
// custom RaftBindAddr to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithRaftBindAddr(raftBindAddr string) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.RaftBindAddr = raftBindAddr
	}
}

// WithRaftBindPort is an option to the NewSugarDB function that allows you to pass a
// custom RaftBindPort to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithRaftBindPort(raftBindPort uint16) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.RaftBindPort = raftBindPort
	}
}

// WithRaftAdvertiseAddr is an option to the NewSugarDB function that allows you to pass a
// custom RaftAdvertiseAddr to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithRaftAdvertiseAddr(raftAdvertiseAddr string) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.RaftAdvertiseAddr = raftAdvertiseAddr
	}
}

// WithRaftAdvertisePort is an option to the NewSugarDB function that allows you to pass a
// custom RaftAdvertisePort to SugarDB.
// If not specified, SugarDB will use the default configuration from config.DefaultConfig().
func WithRaftAdvertisePort(raftAdvertisePort uint16) func(sugardb *SugarDB) {
	return func(sugardb *SugarDB) {
		sugardb.config.RaftAdvertisePort = raftAdvertisePort
	}
}
