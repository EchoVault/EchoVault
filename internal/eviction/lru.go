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

package eviction

import (
	"container/heap"
	"slices"
	"time"
)

type EntryLRU struct {
	key      string // The key, matching the key in the store
	unixTime int64  // Unix time in milliseconds when this key was accessed
	index    int    // The index of the entry in the heap
}

type CacheLRU struct {
	keys    map[string]bool
	entries []*EntryLRU
}

func NewCacheLRU() *CacheLRU {
	cache := CacheLRU{
		keys:    make(map[string]bool),
		entries: make([]*EntryLRU, 0),
	}
	heap.Init(&cache)
	return &cache
}

func (cache *CacheLRU) Len() int {
	return len(cache.entries)
}

func (cache *CacheLRU) Less(i, j int) bool {
	return cache.entries[i].unixTime > cache.entries[j].unixTime
}

func (cache *CacheLRU) Swap(i, j int) {
	cache.entries[i], cache.entries[j] = cache.entries[j], cache.entries[i]
	cache.entries[i].index = i
	cache.entries[j].index = j
}

func (cache *CacheLRU) Push(key any) {
	n := len(cache.entries)
	cache.entries = append(cache.entries, &EntryLRU{
		key:      key.(string),
		unixTime: time.Now().UnixMilli(),
		index:    n,
	})
}

func (cache *CacheLRU) Pop() any {
	old := cache.entries
	n := len(old)
	entry := old[n-1]
	old[n-1] = nil
	entry.index = -1
	cache.entries = old[0 : n-1]
	delete(cache.keys, entry.key)
	return entry.key
}

func (cache *CacheLRU) Update(key string) {
	// If the key does not already exist in the cache, then push it
	if !cache.contains(key) {
		heap.Push(cache, key)
	}
	// Get the item with key
	entryIdx := slices.IndexFunc(cache.entries, func(e *EntryLRU) bool {
		return e.key == key
	})
	entry := cache.entries[entryIdx]
	entry.unixTime = time.Now().UnixMilli()
	heap.Fix(cache, entryIdx)
}

func (cache *CacheLRU) Delete(key string) {
	entryIdx := slices.IndexFunc(cache.entries, func(entry *EntryLRU) bool {
		return entry.key == key
	})
	if entryIdx > -1 {
		heap.Remove(cache, cache.entries[entryIdx].index)
	}
}

func (cache *CacheLRU) contains(key string) bool {
	_, ok := cache.keys[key]
	return ok
}
