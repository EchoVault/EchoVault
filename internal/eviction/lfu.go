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

type EntryLFU struct {
	key       string // The key, matching the key in the store
	count     int    // The number of times this key has been accessed
	addedTime int64  // The time this entry was added to the cache in unix milliseconds
	index     int    // The index of the entry in the heap
}

type CacheLFU struct {
	keys    map[string]bool
	entries []*EntryLFU
}

func NewCacheLFU() *CacheLFU {
	cache := CacheLFU{
		keys:    make(map[string]bool),
		entries: make([]*EntryLFU, 0),
	}
	heap.Init(&cache)
	return &cache
}

func (cache *CacheLFU) Len() int {
	return len(cache.entries)
}

func (cache *CacheLFU) Less(i, j int) bool {
	// If 2 entries have the same count, swap using addedTime
	if cache.entries[i].count == cache.entries[j].count {
		return cache.entries[i].addedTime > cache.entries[j].addedTime
	}
	// Otherwise, swap using count
	return cache.entries[i].count < cache.entries[j].count
}

func (cache *CacheLFU) Swap(i, j int) {
	cache.entries[i], cache.entries[j] = cache.entries[j], cache.entries[i]
	cache.entries[i].index = i
	cache.entries[j].index = j
}

func (cache *CacheLFU) Push(key any) {
	n := len(cache.entries)
	cache.entries = append(cache.entries, &EntryLFU{
		key:       key.(string),
		count:     1,
		addedTime: time.Now().UnixMilli(),
		index:     n,
	})
	cache.keys[key.(string)] = true
}

func (cache *CacheLFU) Pop() any {
	old := cache.entries
	n := len(old)
	entry := old[n-1]
	old[n-1] = nil
	entry.index = -1
	cache.entries = old[0 : n-1]
	delete(cache.keys, entry.key)
	return entry.key
}

func (cache *CacheLFU) Update(key string) {
	// If the key is not contained in the cache, push it.
	if !cache.contains(key) {
		heap.Push(cache, key)
		return
	}
	// Get the item with key
	entryIdx := slices.IndexFunc(cache.entries, func(e *EntryLFU) bool {
		return e.key == key
	})
	entry := cache.entries[entryIdx]
	entry.count += 1
	heap.Fix(cache, entryIdx)
}

func (cache *CacheLFU) Delete(key string) {
	entryIdx := slices.IndexFunc(cache.entries, func(entry *EntryLFU) bool {
		return entry.key == key
	})
	if entryIdx > -1 {
		heap.Remove(cache, cache.entries[entryIdx].index)
	}
}

func (cache *CacheLFU) contains(key string) bool {
	_, ok := cache.keys[key]
	return ok
}
