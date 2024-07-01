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
	"container/heap"
	"context"
	"errors"
	"fmt"
	"github.com/echovault/echovault/internal"
	"github.com/echovault/echovault/internal/constants"
	"github.com/echovault/echovault/internal/eviction"
	"log"
	"math/rand"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

// SwapDBs swaps every TCP client connection from database1 over to database2.
// It also swaps every TCP client connection from database2 over to database1.
// This only affects TCP connections, it does not swap the logical database currently
// being used by the embedded API.
func (server *EchoVault) SwapDBs(database1, database2 int) {
	// If the databases are the same, skip the swap.
	if database1 == database2 {
		return
	}

	// If any of the databases does not exist, create them.
	server.storeLock.Lock()
	for _, database := range []int{database1, database2} {
		if server.store[database] == nil {
			server.createDatabase(database)
		}
	}
	server.storeLock.Unlock()

	// Swap the connections for each database.
	server.connInfo.mut.Lock()
	defer server.connInfo.mut.Unlock()
	for connection, info := range server.connInfo.tcpClients {
		switch info.Database {
		case database1:
			server.connInfo.tcpClients[connection] = internal.ConnectionInfo{
				Id:       info.Id,
				Name:     info.Name,
				Protocol: info.Protocol,
				Database: database2,
			}
		case database2:
			server.connInfo.tcpClients[connection] = internal.ConnectionInfo{
				Id:       info.Id,
				Name:     info.Name,
				Protocol: info.Protocol,
				Database: database1,
			}
		}
	}
}

// Flush flushes all the data from the database at the specified index.
// When -1 is passed, all the logical databases are cleared.
func (server *EchoVault) Flush(database int) {
	server.storeLock.Lock()
	defer server.storeLock.Unlock()

	server.keysWithExpiry.rwMutex.Lock()
	defer server.keysWithExpiry.rwMutex.Unlock()

	server.lfuCache.mutex.Lock()
	defer server.lfuCache.mutex.Unlock()

	server.lruCache.mutex.Lock()
	defer server.lruCache.mutex.Unlock()

	if database == -1 {
		for db, _ := range server.store {
			// Clear db store.
			clear(server.store[db])
			// Clear db volatile key tracker.
			clear(server.keysWithExpiry.keys[db])
			// Clear db LFU cache.
			server.lfuCache.cache[db] = eviction.NewCacheLFU()
			// Clear db LRU cache.
			server.lruCache.cache[db] = eviction.NewCacheLRU()
		}
		return
	}

	// Clear db store.
	clear(server.store[database])
	// Clear db volatile key tracker.
	clear(server.keysWithExpiry.keys[database])
	// Clear db LFU cache.
	server.lfuCache.cache[database] = eviction.NewCacheLFU()
	// Clear db LRU cache.
	server.lruCache.cache[database] = eviction.NewCacheLRU()
}

func (server *EchoVault) keysExist(ctx context.Context, keys []string) map[string]bool {
	server.storeLock.RLock()
	defer server.storeLock.RUnlock()

	database := ctx.Value("Database").(int)

	exists := make(map[string]bool, len(keys))

	for _, key := range keys {
		_, ok := server.store[database][key]
		exists[key] = ok
	}

	return exists
}

func (server *EchoVault) getExpiry(ctx context.Context, key string) time.Time {
	server.storeLock.RLock()
	defer server.storeLock.RUnlock()

	database := ctx.Value("Database").(int)

	entry, ok := server.store[database][key]
	if !ok {
		return time.Time{}
	}

	return entry.ExpireAt
}

func (server *EchoVault) getValues(ctx context.Context, keys []string) map[string]interface{} {
	server.storeLock.Lock()
	defer server.storeLock.Unlock()

	database := ctx.Value("Database").(int)

	values := make(map[string]interface{}, len(keys))

	for _, key := range keys {
		entry, ok := server.store[database][key]
		if !ok {
			values[key] = nil
			continue
		}

		if entry.ExpireAt != (time.Time{}) && entry.ExpireAt.Before(server.clock.Now()) {
			if !server.isInCluster() {
				// If in standalone mode, delete the key directly.
				err := server.deleteKey(ctx, key)
				if err != nil {
					log.Printf("keyExists: %+v\n", err)
				}
			} else if server.isInCluster() && server.raft.IsRaftLeader() {
				// If we're in a raft cluster, and we're the leader, send command to delete the key in the cluster.
				err := server.raftApplyDeleteKey(ctx, key)
				if err != nil {
					log.Printf("keyExists: %+v\n", err)
				}
			} else if server.isInCluster() && !server.raft.IsRaftLeader() {
				// Forward message to leader to initiate key deletion.
				// This is always called regardless of ForwardCommand config value
				// because we always want to remove expired keys.
				server.memberList.ForwardDeleteKey(ctx, key)
			}
			values[key] = nil
			continue
		}

		values[key] = entry.Value
	}

	// Asynchronously update the keys in the cache.
	go func(ctx context.Context, keys []string) {
		if err := server.updateKeysInCache(ctx, keys); err != nil {
			log.Printf("getValues error: %+v\n", err)
		}
	}(ctx, keys)

	return values
}

func (server *EchoVault) setValues(ctx context.Context, entries map[string]interface{}) error {
	server.storeLock.Lock()
	defer server.storeLock.Unlock()

	if internal.IsMaxMemoryExceeded(server.config.MaxMemory) && server.config.EvictionPolicy == constants.NoEviction {
		return errors.New("max memory reached, key value not set")
	}

	database := ctx.Value("Database").(int)

	// If database does not exist, create it.
	if server.store[database] == nil {
		server.createDatabase(database)
	}

	for key, value := range entries {
		expireAt := time.Time{}
		if _, ok := server.store[database][key]; ok {
			expireAt = server.store[database][key].ExpireAt
		}
		server.store[database][key] = internal.KeyData{
			Value:    value,
			ExpireAt: expireAt,
		}
		if !server.isInCluster() {
			server.snapshotEngine.IncrementChangeCount()
		}
	}

	// Asynchronously update the keys in the cache.
	go func(ctx context.Context, entries map[string]interface{}) {
		for key, _ := range entries {
			err := server.updateKeysInCache(ctx, []string{key})
			if err != nil {
				log.Printf("setValues error: %+v\n", err)
			}
		}
	}(ctx, entries)

	return nil
}

func (server *EchoVault) setExpiry(ctx context.Context, key string, expireAt time.Time, touch bool) {
	server.storeLock.Lock()
	defer server.storeLock.Unlock()

	database := ctx.Value("Database").(int)

	server.store[database][key] = internal.KeyData{
		Value:    server.store[database][key].Value,
		ExpireAt: expireAt,
	}

	// If the slice of keys associated with expiry time does not contain the current key, add the key.
	server.keysWithExpiry.rwMutex.Lock()
	if !slices.Contains(server.keysWithExpiry.keys[database], key) {
		server.keysWithExpiry.keys[database] = append(server.keysWithExpiry.keys[database], key)
	}
	server.keysWithExpiry.rwMutex.Unlock()

	// If touch is true, update the keys status in the cache.
	if touch {
		go func(ctx context.Context, key string) {
			err := server.updateKeysInCache(ctx, []string{key})
			if err != nil {
				log.Printf("setExpiry error: %+v\n", err)
			}
		}(ctx, key)
	}
}

func (server *EchoVault) deleteKey(ctx context.Context, key string) error {
	database := ctx.Value("Database").(int)

	// Delete the key from keyLocks and store.
	delete(server.store[database], key)

	// Remove key from slice of keys associated with expiry.
	server.keysWithExpiry.rwMutex.Lock()
	defer server.keysWithExpiry.rwMutex.Unlock()
	server.keysWithExpiry.keys[database] = slices.DeleteFunc(server.keysWithExpiry.keys[database], func(k string) bool {
		return k == key
	})

	// Remove the key from the cache associated with the database.
	switch {
	case slices.Contains([]string{constants.AllKeysLFU, constants.VolatileLFU}, server.config.EvictionPolicy):
		server.lfuCache.cache[database].Delete(key)
	case slices.Contains([]string{constants.AllKeysLRU, constants.VolatileLRU}, server.config.EvictionPolicy):
		server.lruCache.cache[database].Delete(key)
	}

	log.Printf("deleted key %s\n", key)

	return nil
}

func (server *EchoVault) createDatabase(database int) {
	// Create database store.
	server.store[database] = make(map[string]internal.KeyData)

	// Set volatile keys tracker for database.
	server.keysWithExpiry.rwMutex.Lock()
	defer server.keysWithExpiry.rwMutex.Unlock()
	server.keysWithExpiry.keys[database] = make([]string, 0)

	// Create database LFU cache.
	server.lfuCache.mutex.Lock()
	defer server.lfuCache.mutex.Unlock()
	server.lfuCache.cache[database] = eviction.NewCacheLFU()

	// Create database LRU cache.
	server.lruCache.mutex.Lock()
	defer server.lruCache.mutex.Unlock()
	server.lruCache.cache[database] = eviction.NewCacheLRU()
}

func (server *EchoVault) getState() map[int]map[string]interface{} {
	// Wait unit there's no state mutation or copy in progress before starting a new copy process.
	for {
		if !server.stateCopyInProgress.Load() && !server.stateMutationInProgress.Load() {
			server.stateCopyInProgress.Store(true)
			break
		}
	}
	data := make(map[int]map[string]interface{})
	for db, store := range server.store {
		data[db] = make(map[string]interface{})
		for k, v := range store {
			data[db][k] = v
		}
	}
	server.stateCopyInProgress.Store(false)
	return data
}

// updateKeysInCache updates either the key access count or the most recent access time in the cache
// depending on whether an LFU or LRU strategy was used.
func (server *EchoVault) updateKeysInCache(ctx context.Context, keys []string) error {
	database := ctx.Value("Database").(int)
	for _, key := range keys {
		// Only update cache when in standalone mode or when raft leader.
		if server.isInCluster() || (server.isInCluster() && !server.raft.IsRaftLeader()) {
			return nil
		}
		// If max memory is 0, there's no max so no need to update caches.
		if server.config.MaxMemory == 0 {
			return nil
		}
		switch strings.ToLower(server.config.EvictionPolicy) {
		case constants.AllKeysLFU:
			server.lfuCache.mutex.Lock()
			server.lfuCache.cache[database].Update(key)
			server.lfuCache.mutex.Unlock()
		case constants.AllKeysLRU:
			server.lruCache.mutex.Lock()
			server.lruCache.cache[database].Update(key)
			server.lruCache.mutex.Unlock()
		case constants.VolatileLFU:
			server.lfuCache.mutex.Lock()
			if server.store[database][key].ExpireAt != (time.Time{}) {
				server.lfuCache.cache[database].Update(key)
			}
			server.lfuCache.mutex.Unlock()
		case constants.VolatileLRU:
			server.lruCache.mutex.Lock()
			if server.store[database][key].ExpireAt != (time.Time{}) {
				server.lruCache.cache[database].Update(key)
			}
			server.lruCache.mutex.Unlock()
		}
	}

	server.storeLock.Lock()
	defer server.storeLock.Unlock()

	wg := sync.WaitGroup{}
	errChan := make(chan error)
	doneChan := make(chan struct{})

	for db, _ := range server.store {
		wg.Add(1)
		ctx := context.WithValue(ctx, "Database", db)
		go func(ctx context.Context, database int, wg *sync.WaitGroup, errChan *chan error) {
			if err := server.adjustMemoryUsage(ctx); err != nil {
				*errChan <- fmt.Errorf("adjustMemoryUsade database %d", database)
			}
			wg.Done()
		}(ctx, db, &wg, &errChan)
	}

	go func() {
		wg.Wait()
		doneChan <- struct{}{}
	}()

	select {
	case err := <-errChan:
		return fmt.Errorf("adjustMemoryUsage error: %+v", err)
	case <-doneChan:
	}

	return nil
}

// adjustMemoryUsage should only be called from standalone echovault or from raft cluster leader.
func (server *EchoVault) adjustMemoryUsage(ctx context.Context) error {
	// If max memory is 0, there's no need to adjust memory usage.
	if server.config.MaxMemory == 0 {
		return nil
	}

	database := ctx.Value("Database").(int)

	// Check if memory usage is above max-memory.
	// If it is, pop items from the cache until we get under the limit.
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	// If we're using less memory than the max-memory, there's no need to evict.
	if memStats.HeapInuse < server.config.MaxMemory {
		return nil
	}
	// Force a garbage collection first before we start evicting keys.
	runtime.GC()
	runtime.ReadMemStats(&memStats)
	if memStats.HeapInuse < server.config.MaxMemory {
		return nil
	}
	// We've done a GC, but we're still at or above the max memory limit.
	// Start a loop that evicts keys until either the heap is empty or
	// we're below the max memory limit.
	switch {
	case slices.Contains([]string{constants.AllKeysLFU, constants.VolatileLFU}, strings.ToLower(server.config.EvictionPolicy)):
		// Remove keys from LFU cache until we're below the max memory limit or
		// until the LFU cache is empty.
		server.lfuCache.mutex.Lock()
		defer server.lfuCache.mutex.Unlock()
		for {
			// Return if cache is empty
			if server.lfuCache.cache[database].Len() == 0 {
				return fmt.Errorf("adjustMemoryUsage -> LFU cache empty")
			}

			key := heap.Pop(server.lfuCache.cache[database]).(string)
			if !server.isInCluster() {
				// If in standalone mode, directly delete the key
				if err := server.deleteKey(ctx, key); err != nil {
					return fmt.Errorf("adjustMemoryUsage -> LFU cache eviction: %+v", err)
				}
			} else if server.isInCluster() && server.raft.IsRaftLeader() {
				// If in raft cluster, send command to delete key from cluster
				if err := server.raftApplyDeleteKey(ctx, key); err != nil {
					return fmt.Errorf("adjustMemoryUsage -> LFU cache eviction: %+v", err)
				}
			}
			// Run garbage collection
			runtime.GC()
			// Return if we're below max memory
			runtime.ReadMemStats(&memStats)
			if memStats.HeapInuse < server.config.MaxMemory {
				return nil
			}
		}
	case slices.Contains([]string{constants.AllKeysLRU, constants.VolatileLRU}, strings.ToLower(server.config.EvictionPolicy)):
		// Remove keys from th LRU cache until we're below the max memory limit or
		// until the LRU cache is empty.
		server.lruCache.mutex.Lock()
		defer server.lruCache.mutex.Unlock()
		for {
			// Return if cache is empty
			if server.lruCache.cache[database].Len() == 0 {
				return fmt.Errorf("adjustMemoryUsage -> LRU cache empty")
			}

			key := heap.Pop(server.lruCache.cache[database]).(string)
			if !server.isInCluster() {
				// If in standalone mode, directly delete the key.
				if err := server.deleteKey(ctx, key); err != nil {
					return fmt.Errorf("adjustMemoryUsage -> LRU cache eviction: %+v", err)
				}
			} else if server.isInCluster() && server.raft.IsRaftLeader() {
				// If in cluster mode and the node is a cluster leader,
				// send command to delete the key from the cluster.
				if err := server.raftApplyDeleteKey(ctx, key); err != nil {
					return fmt.Errorf("adjustMemoryUsage -> LRU cache eviction: %+v", err)
				}
			}

			// Run garbage collection
			runtime.GC()
			// Return if we're below max memory
			runtime.ReadMemStats(&memStats)
			if memStats.HeapInuse < server.config.MaxMemory {
				return nil
			}
		}
	case slices.Contains([]string{constants.AllKeysRandom}, strings.ToLower(server.config.EvictionPolicy)):
		// Remove random keys until we're below the max memory limit
		// or there are no more keys remaining.
		for {
			// If there are no keys, return error
			if len(server.store) == 0 {
				err := errors.New("no keys to evict")
				return fmt.Errorf("adjustMemoryUsage -> all keys random: %+v", err)
			}
			// Get random key in the database
			idx := rand.Intn(len(server.store))
			for db, data := range server.store {
				if db == database {
					for key, _ := range data {
						if idx == 0 {
							if !server.isInCluster() {
								// If in standalone mode, directly delete the key
								if err := server.deleteKey(ctx, key); err != nil {
									return fmt.Errorf("adjustMemoryUsage -> all keys random: %+v", err)
								}
							} else if server.isInCluster() && server.raft.IsRaftLeader() {
								if err := server.raftApplyDeleteKey(ctx, key); err != nil {
									return fmt.Errorf("adjustMemoryUsage -> all keys random: %+v", err)
								}
							}
							// Run garbage collection
							runtime.GC()
							// Return if we're below max memory
							runtime.ReadMemStats(&memStats)
							if memStats.HeapInuse < server.config.MaxMemory {
								return nil
							}
						}
						idx--
					}
				}
			}
		}
	case slices.Contains([]string{constants.VolatileRandom}, strings.ToLower(server.config.EvictionPolicy)):
		// Remove random keys with an associated expiry time until we're below the max memory limit
		// or there are no more keys with expiry time.
		for {
			// Get random volatile key
			server.keysWithExpiry.rwMutex.RLock()
			idx := rand.Intn(len(server.keysWithExpiry.keys))
			key := server.keysWithExpiry.keys[database][idx]
			server.keysWithExpiry.rwMutex.RUnlock()

			if !server.isInCluster() {
				// If in standalone mode, directly delete the key
				if err := server.deleteKey(ctx, key); err != nil {
					return fmt.Errorf("adjustMemoryUsage -> volatile keys random: %+v", err)
				}
			} else if server.isInCluster() && server.raft.IsRaftLeader() {
				if err := server.raftApplyDeleteKey(ctx, key); err != nil {
					return fmt.Errorf("adjustMemoryUsage -> volatile keys randome: %+v", err)
				}
			}

			// Run garbage collection
			runtime.GC()
			// Return if we're below max memory
			runtime.ReadMemStats(&memStats)
			if memStats.HeapInuse < server.config.MaxMemory {
				return nil
			}
		}
	default:
		return nil
	}
}

// evictKeysWithExpiredTTL is a function that samples keys with an associated TTL
// and evicts keys that are currently expired.
// This function will sample 20 keys from the list of keys with an associated TTL,
// if the key is expired, it will be evicted.
// This function is only executed in standalone mode or by the raft cluster leader.
func (server *EchoVault) evictKeysWithExpiredTTL(ctx context.Context) error {
	// Only execute this if we're in standalone mode, or raft cluster leader.
	if server.isInCluster() && !server.raft.IsRaftLeader() {
		return nil
	}

	server.keysWithExpiry.rwMutex.RLock()

	database := ctx.Value("Database").(int)

	// Sample size should be the configured sample size, or the size of the keys with expiry,
	// whichever one is smaller.
	sampleSize := int(server.config.EvictionSample)
	if len(server.keysWithExpiry.keys[database]) < sampleSize {
		sampleSize = len(server.keysWithExpiry.keys)
	}
	keys := make([]string, sampleSize)

	deletedCount := 0
	thresholdPercentage := 20

	var idx int
	var key string
	for i := 0; i < len(keys); i++ {
		for {
			// Retry retrieval of a random key until we find a key that is not already in the list of sampled keys.
			idx = rand.Intn(len(server.keysWithExpiry.keys))
			key = server.keysWithExpiry.keys[database][idx]
			if !slices.Contains(keys, key) {
				keys[i] = key
				break
			}
		}
	}
	server.keysWithExpiry.rwMutex.RUnlock()

	// Loop through the keys and delete them if they're expired
	server.storeLock.Lock()
	defer server.storeLock.Unlock()
	for _, k := range keys {
		// Delete the expired key
		deletedCount += 1
		if !server.isInCluster() {
			if err := server.deleteKey(ctx, k); err != nil {
				return fmt.Errorf("evictKeysWithExpiredTTL -> standalone delete: %+v", err)
			}
		} else if server.isInCluster() && server.raft.IsRaftLeader() {
			if err := server.raftApplyDeleteKey(ctx, k); err != nil {
				return fmt.Errorf("evictKeysWithExpiredTTL -> cluster delete: %+v", err)
			}
		}
	}

	// If sampleSize is 0, there's no need to calculate deleted percentage.
	if sampleSize == 0 {
		return nil
	}

	log.Printf("%d keys sampled, %d keys deleted\n", sampleSize, deletedCount)

	// If the deleted percentage is over 20% of the sample size, execute the function again immediately.
	if (deletedCount/sampleSize)*100 >= thresholdPercentage {
		log.Printf("deletion ratio (%d percent) reached threshold (%d percent), sampling again\n",
			(deletedCount/sampleSize)*100, thresholdPercentage)
		return server.evictKeysWithExpiredTTL(ctx)
	}

	return nil
}
