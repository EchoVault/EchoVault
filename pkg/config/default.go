package config

import (
	"time"

	"github.com/echovault/sugardb/internal"
	"github.com/echovault/sugardb/internal/constants"
)

func DefaultConfig() Config {
	raftBindAddr, _ := internal.GetIPAddress()
	raftBindPort, _ := internal.GetFreePort()

	return Config{
		TLS:               false,
		MTLS:              false,
		CertKeyPairs:      make([][]string, 0),
		ClientCAs:         make([]string, 0),
		Port:              7480,
		ServerID:          "",
		JoinAddr:          "",
		BindAddr:          "localhost",
		RaftBindAddr:      raftBindAddr,
		RaftBindPort:      uint16(raftBindPort),
		DiscoveryPort:     7946,
		DataDir:           ".",
		BootstrapCluster:  false,
		AclConfig:         "",
		ForwardCommand:    false,
		RequirePass:       false,
		Password:          "",
		SnapShotThreshold: 1000,
		SnapshotInterval:  5 * time.Minute,
		RestoreAOF:        false,
		RestoreSnapshot:   false,
		AOFSyncStrategy:   "everysec",
		MaxMemory:         0,
		EvictionPolicy:    constants.NoEviction,
		EvictionSample:    20,
		EvictionInterval:  100 * time.Millisecond,
		ElectionTimeout:   1000 * time.Millisecond,
		HeartbeatTimeout:  1000 * time.Millisecond,
		CommitTimeout:     50 * time.Millisecond,
		Modules:           make([]string, 0),
	}
}
