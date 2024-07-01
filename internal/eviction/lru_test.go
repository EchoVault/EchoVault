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
	"testing"
	"time"
)

func Test_CacheLRU(t *testing.T) {
	keys := []string{"key1", "key2", "key3", "key4", "key5"}

	cache := eviction.NewCacheLRU()

	for _, key := range keys {
		cache.Update(key)
	}

	access := []string{"key3", "key4", "key1", "key2", "key5"}
	ticker := time.NewTicker(200 * time.Millisecond)
	for _, key := range access {
		cache.Update(key)
		// Yield
		<-ticker.C
	}
	ticker.Stop()

	for i := len(access) - 1; i >= 0; i-- {
		key := heap.Pop(cache).(string)
		if key != access[i] {
			t.Errorf("expected key at index %d to be %s, got %s", i, access[i], key)
		}
	}
}
