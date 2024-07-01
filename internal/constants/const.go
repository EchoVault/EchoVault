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

package constants

const Version = "0.8.0" // Next EchoVault version. Update this before each release.

const (
	ACLModule        = "acl"
	AdminModule      = "admin"
	ConnectionModule = "connection"
	GenericModule    = "generic"
	HashModule       = "hash"
	ListModule       = "list"
	PubSubModule     = "pubsub"
	SetModule        = "set"
	SortedSetModule  = "sortedset"
	StringModule     = "string"
)

const (
	AdminCategory       = "admin"
	BitmapCategory      = "bitmap"
	BlockingCategory    = "blocking"
	ConnectionCategory  = "connection"
	DangerousCategory   = "dangerous"
	GeoCategory         = "geo"
	HashCategory        = "hash"
	HyperLogLogCategory = "hyperloglog"
	FastCategory        = "fast"
	KeyspaceCategory    = "keyspace"
	ListCategory        = "list"
	PubSubCategory      = "pubsub"
	ReadCategory        = "read"
	ScriptingCategory   = "scripting"
	SetCategory         = "set"
	SortedSetCategory   = "sortedset"
	SlowCategory        = "slow"
	StreamCategory      = "stream"
	StringCategory      = "string"
	TransactionCategory = "transaction"
	WriteCategory       = "write"
)

const (
	OkResponse        = "+OK\r\n"
	WrongArgsResponse = "wrong number of arguments"
)

const (
	NoEviction     = "noeviction"
	AllKeysLRU     = "allkeys-lru"
	AllKeysLFU     = "allkeys-lfu"
	VolatileLRU    = "volatile-lru"
	VolatileLFU    = "volatile-lfu"
	AllKeysRandom  = "allkeys-random"
	VolatileRandom = "volatile-random"
)
