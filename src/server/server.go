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

package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/echovault/echovault/src/aof"
	"github.com/echovault/echovault/src/eviction"
	"github.com/echovault/echovault/src/memberlist"
	"github.com/echovault/echovault/src/raft"
	"github.com/echovault/echovault/src/snapshot"
	"github.com/echovault/echovault/src/utils"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type Server struct {
	// Config holds the server configuration variables.
	Config utils.Config

	// The current index for the latest connection id.
	// This number is incremented everytime there's a new connection and
	// the new number is the new connection's ID.
	ConnID atomic.Uint64

	store           map[string]utils.KeyData // Data store to hold the keys and their associated data, expiry time, etc.
	keyLocks        map[string]*sync.RWMutex // Map to hold all the individual key locks.
	keyCreationLock *sync.Mutex              // The mutex for creating a new key. Only one goroutine should be able to create a key at a time.

	// Holds all the keys that are currently associated with an expiry.
	keysWithExpiry struct {
		rwMutex sync.RWMutex // Mutex as only one process should be able to update this list at a time.
		keys    []string     // string slice of the volatile keys
	}
	// LFU cache used when eviction policy is allkeys-lfu or volatile-lfu
	lfuCache struct {
		mutex sync.Mutex        // Mutex as only one goroutine can edit the LFU cache at a time.
		cache eviction.CacheLFU // LFU cache represented by a min head.
	}
	// LRU cache used when eviction policy is allkeys-lru or volatile-lru
	lruCache struct {
		mutex sync.Mutex        // Mutex as only one goroutine can edit the LRU at a time.
		cache eviction.CacheLRU // LRU cache represented by a max head.
	}

	// Holds the list of all commands supported by the server.
	Commands []utils.Command

	raft       *raft.Raft             // The raft replication layer for the server.
	memberList *memberlist.MemberList // The memberlist layer for the server.

	CancelCh *chan os.Signal

	ACL    utils.ACL
	PubSub utils.PubSub

	SnapshotInProgress         atomic.Bool      // Atomic boolean that's true when actively taking a snapshot.
	RewriteAOFInProgress       atomic.Bool      // Atomic boolean that's true when actively rewriting AOF file is in progress.
	StateCopyInProgress        atomic.Bool      // Atomic boolean that's true when actively copying state for snapshotting or preamble generation.
	StateMutationInProgress    atomic.Bool      // Atomic boolean that is set to true when state mutation is in progress.
	LatestSnapshotMilliseconds atomic.Int64     // Unix epoch in milliseconds
	SnapshotEngine             *snapshot.Engine // Snapshot engine for standalone mode
	AOFEngine                  *aof.Engine      // AOF engine for standalone mode
}

type Opts struct {
	Config   utils.Config
	ACL      utils.ACL
	PubSub   utils.PubSub
	CancelCh *chan os.Signal
	Commands []utils.Command
}

func NewServer(opts Opts) *Server {
	server := &Server{
		Config:          opts.Config,
		ACL:             opts.ACL,
		PubSub:          opts.PubSub,
		CancelCh:        opts.CancelCh,
		Commands:        opts.Commands,
		store:           make(map[string]utils.KeyData),
		keyLocks:        make(map[string]*sync.RWMutex),
		keyCreationLock: &sync.Mutex{},
	}
	if server.IsInCluster() {
		server.raft = raft.NewRaft(raft.Opts{
			Config:     opts.Config,
			Server:     server,
			GetCommand: server.getCommand,
			DeleteKey:  server.DeleteKey,
		})
		server.memberList = memberlist.NewMemberList(memberlist.Opts{
			Config:           opts.Config,
			HasJoinedCluster: server.raft.HasJoinedCluster,
			AddVoter:         server.raft.AddVoter,
			RemoveRaftServer: server.raft.RemoveServer,
			IsRaftLeader:     server.raft.IsRaftLeader,
			ApplyMutate:      server.raftApplyCommand,
			ApplyDeleteKey:   server.raftApplyDeleteKey,
		})
	} else {
		// Set up standalone snapshot engine
		server.SnapshotEngine = snapshot.NewSnapshotEngine(snapshot.Opts{
			Config:                        opts.Config,
			StartSnapshot:                 server.StartSnapshot,
			FinishSnapshot:                server.FinishSnapshot,
			GetState:                      server.GetState,
			SetLatestSnapshotMilliseconds: server.SetLatestSnapshot,
			GetLatestSnapshotMilliseconds: server.GetLatestSnapshot,
			SetValue: func(key string, value interface{}) error {
				ctx := context.Background()
				if _, err := server.CreateKeyAndLock(ctx, key); err != nil {
					return err
				}
				if err := server.SetValue(ctx, key, value); err != nil {
					return err
				}
				server.KeyUnlock(ctx, key)
				return nil
			},
			SetExpiry: func(key string, expireAt time.Time) error {
				ctx := context.Background()
				if _, err := server.KeyLock(ctx, key); err != nil {
					return err
				}
				server.SetExpiry(ctx, key, expireAt, false)
				server.KeyUnlock(ctx, key)
				return nil
			},
		})
		// Set up standalone AOF engine
		server.AOFEngine = aof.NewAOFEngine(
			aof.WithDirectory(opts.Config.DataDir),
			aof.WithStrategy(opts.Config.AOFSyncStrategy),
			aof.WithStartRewriteFunc(server.StartRewriteAOF),
			aof.WithFinishRewriteFunc(server.FinishRewriteAOF),
			aof.WithGetStateFunc(server.GetState),
			aof.WithSetValueFunc(func(key string, value interface{}) error {
				ctx := context.Background()
				if _, err := server.CreateKeyAndLock(ctx, key); err != nil {
					return err
				}
				if err := server.SetValue(ctx, key, value); err != nil {
					return err
				}
				server.KeyUnlock(ctx, key)
				return nil
			}),
			aof.WithSetExpiryFunc(func(key string, expireAt time.Time) error {
				ctx := context.Background()
				if _, err := server.KeyLock(ctx, key); err != nil {
					return err
				}
				server.SetExpiry(ctx, key, expireAt, false)
				server.KeyUnlock(ctx, key)
				return nil
			}),
			aof.WithHandleCommandFunc(func(command []byte) {
				_, err := server.handleCommand(context.Background(), command, nil, true)
				if err != nil {
					log.Println(err)
				}
			}),
		)
	}

	// If eviction policy is not noeviction, start a goroutine to evict keys every 100 milliseconds.
	if server.Config.EvictionPolicy != utils.NoEviction {
		go func() {
			for {
				<-time.After(server.Config.EvictionInterval)
				if err := server.evictKeysWithExpiredTTL(context.Background()); err != nil {
					log.Println(err)
				}
			}
		}()
	}

	return server
}

func (server *Server) StartTCP(ctx context.Context) {
	conf := server.Config

	listenConfig := net.ListenConfig{
		KeepAlive: 200 * time.Millisecond,
	}

	listener, err := listenConfig.Listen(ctx, "tcp", fmt.Sprintf("%s:%d", conf.BindAddr, conf.Port))

	if err != nil {
		log.Fatal(err)
	}

	if !conf.TLS {
		// TCP
		fmt.Printf("Starting TCP server at Address %s, Port %d...\n", conf.BindAddr, conf.Port)
	}

	if conf.TLS || conf.MTLS {
		// TLS
		if conf.TLS {
			fmt.Printf("Starting mTLS server at Address %s, Port %d...\n", conf.BindAddr, conf.Port)
		} else {
			fmt.Printf("Starting TLS server at Address %s, Port %d...\n", conf.BindAddr, conf.Port)
		}

		var certificates []tls.Certificate
		for _, certKeyPair := range conf.CertKeyPairs {
			c, err := tls.LoadX509KeyPair(certKeyPair[0], certKeyPair[1])
			if err != nil {
				log.Fatal(err)
			}
			certificates = append(certificates, c)
		}

		clientAuth := tls.NoClientCert
		clientCerts := x509.NewCertPool()

		if conf.MTLS {
			clientAuth = tls.RequireAndVerifyClientCert
			for _, c := range conf.ClientCAs {
				ca, err := os.Open(c)
				if err != nil {
					log.Fatal(err)
				}
				certBytes, err := io.ReadAll(ca)
				if err != nil {
					log.Fatal(err)
				}
				if ok := clientCerts.AppendCertsFromPEM(certBytes); !ok {
					log.Fatal(err)
				}
			}
		}

		listener = tls.NewListener(listener, &tls.Config{
			Certificates: certificates,
			ClientAuth:   clientAuth,
			ClientCAs:    clientCerts,
		})
	}

	// Listen to connection
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Could not establish connection")
			continue
		}
		// Read loop for connection
		go server.handleConnection(ctx, conn)
	}
}

func (server *Server) handleConnection(ctx context.Context, conn net.Conn) {
	// If ACL module is loaded, register the connection with the ACL
	if server.ACL != nil {
		server.ACL.RegisterConnection(&conn)
	}

	w, r := io.Writer(conn), io.Reader(conn)

	cid := server.ConnID.Add(1)
	ctx = context.WithValue(ctx, utils.ContextConnID("ConnectionID"),
		fmt.Sprintf("%s-%d", ctx.Value(utils.ContextServerID("ServerID")), cid))

	for {
		message, err := utils.ReadMessage(r)

		if err != nil && errors.Is(err, io.EOF) {
			// Connection closed
			log.Println(err)
			break
		}

		if err != nil {
			log.Println(err)
			break
		}

		res, err := server.handleCommand(ctx, message, &conn, false)

		if err != nil && errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			if _, err = w.Write([]byte(fmt.Sprintf("-Error %s\r\n", err.Error()))); err != nil {
				log.Println(err)
			}
			continue
		}

		chunkSize := 1024

		// If the length of the response is 0, return nothing to the client
		if len(res) == 0 {
			continue
		}

		if len(res) <= chunkSize {
			_, _ = w.Write(res)
			continue
		}

		// If the response is large, send it in chunks.
		startIndex := 0
		for {
			// If the current start index is less than chunkSize from length, return the remaining bytes.
			if len(res)-1-startIndex < chunkSize {
				_, err = w.Write(res[startIndex:])
				if err != nil {
					log.Println(err)
				}
				break
			}
			n, _ := w.Write(res[startIndex : startIndex+chunkSize])
			if n < chunkSize {
				break
			}
			startIndex += chunkSize
		}
	}

	if err := conn.Close(); err != nil {
		log.Println(err)
	}
}

func (server *Server) Start(ctx context.Context) {
	conf := server.Config

	if conf.TLS && len(conf.CertKeyPairs) <= 0 {
		log.Fatal("must provide certificate and key file paths for TLS mode")
		return
	}

	if server.IsInCluster() {
		// Initialise raft and memberlist
		server.raft.RaftInit(ctx)
		server.memberList.MemberListInit(ctx)
		if server.raft.IsRaftLeader() {
			server.InitialiseCaches()
		}
	}

	if !server.IsInCluster() {
		server.InitialiseCaches()
		// Restore from AOF by default if it's enabled
		if conf.RestoreAOF {
			err := server.AOFEngine.Restore()
			if err != nil {
				log.Println(err)
			}
		}

		// Restore from snapshot if snapshot restore is enabled and AOF restore is disabled
		if conf.RestoreSnapshot && !conf.RestoreAOF {
			err := server.SnapshotEngine.Restore(ctx)
			if err != nil {
				log.Println(err)
			}
		}
		server.SnapshotEngine.Start(ctx)

	}

	server.StartTCP(ctx)
}

func (server *Server) TakeSnapshot() error {
	if server.SnapshotInProgress.Load() {
		return errors.New("snapshot already in progress")
	}

	go func() {
		if server.IsInCluster() {
			// Handle snapshot in cluster mode
			if err := server.raft.TakeSnapshot(); err != nil {
				log.Println(err)
			}
			return
		}
		// Handle snapshot in standalone mode
		if err := server.SnapshotEngine.TakeSnapshot(); err != nil {
			log.Println(err)
		}
	}()

	return nil
}

func (server *Server) StartSnapshot() {
	server.SnapshotInProgress.Store(true)
}

func (server *Server) FinishSnapshot() {
	server.SnapshotInProgress.Store(false)
}

func (server *Server) SetLatestSnapshot(msec int64) {
	server.LatestSnapshotMilliseconds.Store(msec)
}

func (server *Server) GetLatestSnapshot() int64 {
	return server.LatestSnapshotMilliseconds.Load()
}

func (server *Server) StartRewriteAOF() {
	server.RewriteAOFInProgress.Store(true)
}

func (server *Server) FinishRewriteAOF() {
	server.RewriteAOFInProgress.Store(false)
}

func (server *Server) RewriteAOF() error {
	if server.RewriteAOFInProgress.Load() {
		return errors.New("aof rewrite in progress")
	}
	go func() {
		if err := server.AOFEngine.RewriteLog(); err != nil {
			log.Println(err)
		}
	}()
	return nil
}

func (server *Server) ShutDown(ctx context.Context) {
	if server.IsInCluster() {
		server.raft.RaftShutdown(ctx)
		server.memberList.MemberListShutdown(ctx)
	}
}

func (server *Server) InitialiseCaches() {
	// Set up LFU cache
	server.lfuCache = struct {
		mutex sync.Mutex
		cache eviction.CacheLFU
	}{
		mutex: sync.Mutex{},
		cache: eviction.NewCacheLFU(),
	}
	// set up LRU cache
	server.lruCache = struct {
		mutex sync.Mutex
		cache eviction.CacheLRU
	}{
		mutex: sync.Mutex{},
		cache: eviction.NewCacheLRU(),
	}
	// TODO: If eviction policy is volatile-ttl, start goroutine that continuously reads the mem stats
	// TODO: before triggering purge once max-memory is reached
}
