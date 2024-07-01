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

package eviction_test

import (
	"container/heap"
	"github.com/echovault/echovault/internal/eviction"
	"sync"
	"testing"
)

func Test_CacheLFU(t *testing.T) {
	entries := []struct {
		key    string
		access int
	}{
		{key: "key1", access: 1},
		{key: "key2", access: 5},
		{key: "key5", access: 2},
		{key: "key3", access: 4},
		{key: "key4", access: 3},
	}

	cache := eviction.NewCacheLFU()
	mut := sync.RWMutex{}

	wg := sync.WaitGroup{}
	for _, entry := range entries {
		wg.Add(1)
		go func(entry struct {
			key    string
			access int
		}) {
			for i := 0; i < entry.access; i++ {
				mut.Lock()
				cache.Update(entry.key)
				mut.Unlock()
			}
			wg.Done()
		}(entry)
	}
	wg.Wait()

	expectedKeys := []string{"key1", "key5", "key4", "key3", "key2"}

	mut.Lock()
	for i := 0; i < len(expectedKeys); i++ {
		key := heap.Pop(cache).(string)
		if key != expectedKeys[i] {
			t.Errorf("expected popped key at index %d to be %s, got %s", i, expectedKeys[i], key)
		}
	}
	mut.Unlock()
}
