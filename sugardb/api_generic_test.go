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
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/echovault/sugardb/internal"
	"github.com/echovault/sugardb/internal/clock"
	"github.com/echovault/sugardb/internal/constants"
	"github.com/echovault/sugardb/pkg/config"
)

func TestSugarDB_Generic(t *testing.T) {
	mockClock := clock.NewClock()

	server := createSugarDB()
	t.Cleanup(func() {
		server.ShutDown()
	})

	t.Run("TestSugarDB_DEL", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name         string
			presetValues map[string]internal.KeyData
			keys         []string
			want         int
			wantErr      bool
		}{
			{
				name: "Delete several keys and return deleted count",
				keys: []string{"del_key1", "del_key2", "del_key3", "del_key4", "del_key5"},
				presetValues: map[string]internal.KeyData{
					"del_key1": {Value: "value1", ExpireAt: time.Time{}},
					"del_key2": {Value: "value2", ExpireAt: time.Time{}},
					"del_key3": {Value: "value3", ExpireAt: time.Time{}},
					"del_key4": {Value: "value4", ExpireAt: time.Time{}},
				},
				want:    4,
				wantErr: false,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if tt.presetValues != nil {
					for k, d := range tt.presetValues {
						presetKeyData(server, context.Background(), k, d)
					}
				}
				got, err := server.Del(tt.keys...)
				if (err != nil) != tt.wantErr {
					t.Errorf("DEL() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if got != tt.want {
					t.Errorf("DEL() got = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("TestSugarDB_EXPIRE", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name         string
			presetValues map[string]internal.KeyData
			cmd          string
			key          string
			time         int
			expireOpts   ExpireOptions
			want         bool
			wantErr      bool
		}{
			{
				name:       "Set new expire by seconds",
				cmd:        "EXPIRE",
				key:        "expire_key1",
				time:       100,
				expireOpts: nil,
				presetValues: map[string]internal.KeyData{
					"expire_key1": {Value: "value1", ExpireAt: time.Time{}},
				},
				want:    true,
				wantErr: false,
			},
			{
				name:       "Set new expire by milliseconds",
				cmd:        "PEXPIRE",
				key:        "expire_key2",
				time:       1000,
				expireOpts: nil,
				presetValues: map[string]internal.KeyData{
					"expire_key2": {Value: "value2", ExpireAt: time.Time{}},
				},
				want:    true,
				wantErr: false,
			},
			{
				name:       "Set new expire only when key does not have an expiry time with NX flag",
				cmd:        "EXPIRE",
				key:        "expire_key3",
				time:       1000,
				expireOpts: NX,
				presetValues: map[string]internal.KeyData{
					"expire_key3": {Value: "value3", ExpireAt: time.Time{}},
				},
				want:    true,
				wantErr: false,
			},
			{
				name:       "Return false when NX flag is provided and key already has an expiry time",
				cmd:        "EXPIRE",
				key:        "expire_key4",
				time:       1000,
				expireOpts: NX,
				presetValues: map[string]internal.KeyData{
					"expire_key4": {Value: "value4", ExpireAt: mockClock.Now().Add(1000 * time.Second)},
				},
				want:    false,
				wantErr: false,
			},
			{
				name:       "Set new expire time from now key only when the key already has an expiry time with XX flag",
				cmd:        "EXPIRE",
				key:        "expire_key5",
				time:       1000,
				expireOpts: XX,
				presetValues: map[string]internal.KeyData{
					"expire_key5": {Value: "value5", ExpireAt: mockClock.Now().Add(30 * time.Second)},
				},
				want:    true,
				wantErr: false,
			},
			{
				name:       "Return false when key does not have an expiry and the XX flag is provided",
				cmd:        "EXPIRE",
				time:       1000,
				expireOpts: XX,
				key:        "expire_key6",
				presetValues: map[string]internal.KeyData{
					"expire_key6": {Value: "value6", ExpireAt: time.Time{}},
				},
				want:    false,
				wantErr: false,
			},
			{
				name:       "Set expiry time when the provided time is after the current expiry time when GT flag is provided",
				cmd:        "EXPIRE",
				key:        "expire_key7",
				time:       100000,
				expireOpts: GT,
				presetValues: map[string]internal.KeyData{
					"expire_key7": {Value: "value7", ExpireAt: mockClock.Now().Add(30 * time.Second)},
				},
				want:    true,
				wantErr: false,
			},
			{
				name:       "Return false when GT flag is passed and current expiry time is greater than provided time",
				cmd:        "EXPIRE",
				key:        "expire_key8",
				time:       1000,
				expireOpts: GT,
				presetValues: map[string]internal.KeyData{
					"expire_key8": {Value: "value8", ExpireAt: mockClock.Now().Add(3000 * time.Second)},
				},
				want:    false,
				wantErr: false,
			},
			{
				name:       "Return false when GT flag is passed and key does not have an expiry time",
				cmd:        "EXPIRE",
				key:        "expire_key9",
				time:       1000,
				expireOpts: GT,
				presetValues: map[string]internal.KeyData{
					"expire_key9": {Value: "value9", ExpireAt: time.Time{}},
				},
				want:    false,
				wantErr: false,
			},
			{
				name:       "Set expiry time when the provided time is before the current expiry time when LT flag is provided",
				cmd:        "EXPIRE",
				key:        "expire_key10",
				time:       1000,
				expireOpts: LT,
				presetValues: map[string]internal.KeyData{
					"expire_key10": {Value: "value10", ExpireAt: mockClock.Now().Add(3000 * time.Second)},
				},
				want:    true,
				wantErr: false,
			},
			{
				name:       "Return false when LT flag is passed and current expiry time is less than provided time",
				cmd:        "EXPIRE",
				key:        "expire_key11",
				time:       50000,
				expireOpts: LT,
				presetValues: map[string]internal.KeyData{
					"expire_key11": {Value: "value11", ExpireAt: mockClock.Now().Add(30 * time.Second)},
				},
				want:    false,
				wantErr: false,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if tt.presetValues != nil {
					for k, d := range tt.presetValues {
						presetKeyData(server, context.Background(), k, d)
					}
				}
				var got bool
				var err error
				if strings.EqualFold(tt.cmd, "PEXPIRE") {
					got, err = server.PExpire(tt.key, tt.time, tt.expireOpts)
				} else {
					got, err = server.Expire(tt.key, tt.time, tt.expireOpts)
				}
				if (err != nil) != tt.wantErr {
					t.Errorf("(P)EXPIRE() error = %v, wantErr %v, key %s", err, tt.wantErr, tt.key)
					return
				}
				if got != tt.want {
					t.Errorf("(P)EXPIRE() got = %v, want %v, key %s", got, tt.want, tt.key)
				}
			})
		}
	})

	t.Run("TestSugarDB_EXPIREAT", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name         string
			presetValues map[string]internal.KeyData
			cmd          string
			key          string
			time         int
			expireAtOpts ExpireOptions
			want         int
			wantErr      bool
		}{
			{
				name:         "Set new expire by unix seconds",
				cmd:          "EXPIREAT",
				key:          "expireat_key1",
				expireAtOpts: nil,
				time:         int(mockClock.Now().Add(1000 * time.Second).Unix()),
				presetValues: map[string]internal.KeyData{
					"expireat_key1": {Value: "value1", ExpireAt: time.Time{}},
				},
				want:    1,
				wantErr: false,
			},
			{
				name:         "Set new expire by milliseconds",
				cmd:          "PEXPIREAT",
				key:          "expireat_key2",
				expireAtOpts: nil,
				time:         int(mockClock.Now().Add(1000 * time.Second).UnixMilli()),
				presetValues: map[string]internal.KeyData{
					"expireat_key2": {Value: "value2", ExpireAt: time.Time{}},
				},
				want:    1,
				wantErr: false,
			},
			{ // 3.
				name:         "Set new expire only when key does not have an expiry time with NX flag",
				cmd:          "EXPIREAT",
				key:          "expireat_key3",
				time:         int(mockClock.Now().Add(1000 * time.Second).Unix()),
				expireAtOpts: NX,
				presetValues: map[string]internal.KeyData{
					"expireat_key3": {Value: "value3", ExpireAt: time.Time{}},
				},
				want:    1,
				wantErr: false,
			},
			{
				name:         "Return 0, when NX flag is provided and key already has an expiry time",
				cmd:          "EXPIREAT",
				time:         int(mockClock.Now().Add(1000 * time.Second).Unix()),
				expireAtOpts: NX,
				key:          "expireat_key4",
				presetValues: map[string]internal.KeyData{
					"expireat_key4": {Value: "value4", ExpireAt: mockClock.Now().Add(1000 * time.Second)},
				},
				want:    0,
				wantErr: false,
			},
			{
				name:         "Set new expire time from now key only when the key already has an expiry time with XX flag",
				cmd:          "EXPIREAT",
				time:         int(mockClock.Now().Add(1000 * time.Second).Unix()),
				key:          "expireat_key5",
				expireAtOpts: XX,
				presetValues: map[string]internal.KeyData{
					"expireat_key5": {Value: "value5", ExpireAt: mockClock.Now().Add(30 * time.Second)},
				},
				want:    1,
				wantErr: false,
			},
			{
				name:         "Return 0 when key does not have an expiry and the XX flag is provided",
				cmd:          "EXPIREAT",
				key:          "expireat_key6",
				time:         int(mockClock.Now().Add(1000 * time.Second).Unix()),
				expireAtOpts: XX,
				presetValues: map[string]internal.KeyData{
					"expireat_key6": {Value: "value6", ExpireAt: time.Time{}},
				},
				want:    0,
				wantErr: false,
			},
			{
				name:         "Set expiry time when the provided time is after the current expiry time when GT flag is provided",
				cmd:          "EXPIREAT",
				key:          "expireat_key7",
				time:         int(mockClock.Now().Add(1000 * time.Second).Unix()),
				expireAtOpts: GT,
				presetValues: map[string]internal.KeyData{
					"expireat_key7": {Value: "value7", ExpireAt: mockClock.Now().Add(30 * time.Second)},
				},
				want:    1,
				wantErr: false,
			},
			{
				name:         "Return 0 when GT flag is passed and current expiry time is greater than provided time",
				cmd:          "EXPIREAT",
				key:          "expireat_key8",
				time:         int(mockClock.Now().Add(1000 * time.Second).Unix()),
				expireAtOpts: GT,
				presetValues: map[string]internal.KeyData{
					"expireat_key8": {Value: "value8", ExpireAt: mockClock.Now().Add(3000 * time.Second)},
				},
				want:    0,
				wantErr: false,
			},
			{
				name:         "Return 0 when GT flag is passed and key does not have an expiry time",
				cmd:          "EXPIREAT",
				key:          "expireat_key9",
				time:         int(mockClock.Now().Add(1000 * time.Second).Unix()),
				expireAtOpts: GT,
				presetValues: map[string]internal.KeyData{
					"expireat_key9": {Value: "value9", ExpireAt: time.Time{}},
				},
				want: 0,
			},
			{
				name:         "Set expiry time when the provided time is before the current expiry time when LT flag is provided",
				cmd:          "EXPIREAT",
				key:          "expireat_key10",
				time:         int(mockClock.Now().Add(1000 * time.Second).Unix()),
				expireAtOpts: LT,
				presetValues: map[string]internal.KeyData{
					"expireat_key10": {Value: "value10", ExpireAt: mockClock.Now().Add(3000 * time.Second)},
				},
				want:    1,
				wantErr: false,
			},
			{
				name:         "Return 0 when LT flag is passed and current expiry time is less than provided time",
				cmd:          "EXPIREAT",
				key:          "expireat_key11",
				time:         int(mockClock.Now().Add(3000 * time.Second).Unix()),
				expireAtOpts: LT,
				presetValues: map[string]internal.KeyData{
					"expireat_key11": {Value: "value11", ExpireAt: mockClock.Now().Add(1000 * time.Second)},
				},
				want:    0,
				wantErr: false,
			},
			{
				name:         "Return 0 when LT flag is passed and key does not have an expiry time",
				cmd:          "EXPIREAT",
				key:          "expireat_key12",
				time:         int(mockClock.Now().Add(1000 * time.Second).Unix()),
				expireAtOpts: LT,
				presetValues: map[string]internal.KeyData{
					"expireat_key12": {Value: "value12", ExpireAt: time.Time{}},
				},
				want:    1,
				wantErr: false,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if tt.presetValues != nil {
					for k, d := range tt.presetValues {
						presetKeyData(server, context.Background(), k, d)
					}
				}
				var got int
				var err error
				if strings.EqualFold(tt.cmd, "PEXPIREAT") {
					got, err = server.PExpireAt(tt.key, tt.time, tt.expireAtOpts)
				} else {
					got, err = server.ExpireAt(tt.key, tt.time, tt.expireAtOpts)
				}
				if (err != nil) != tt.wantErr {
					t.Errorf("(P)EXPIREAT() error = %v, wantErr %v, KEY %s", err, tt.wantErr, tt.key)
					return
				}
				if got != tt.want {
					t.Errorf("(P)EXPIREAT() got = %v, want %v, KEY %s", got, tt.want, tt.key)
				}
			})
		}
	})

	t.Run("TestSugarDB_EXPIRETIME", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name           string
			presetValues   map[string]internal.KeyData
			key            string
			expiretimeFunc func(key string) (int, error)
			want           int
			wantErr        bool
		}{
			{
				name: "Return expire time in seconds",
				key:  "expiretime_key1",
				presetValues: map[string]internal.KeyData{
					"expiretime_key1": {Value: "value1", ExpireAt: mockClock.Now().Add(100 * time.Second)},
				},
				expiretimeFunc: server.ExpireTime,
				want:           int(mockClock.Now().Add(100 * time.Second).Unix()),
				wantErr:        false,
			},
			{
				name: "Return expire time in milliseconds",
				key:  "expiretime_key2",
				presetValues: map[string]internal.KeyData{
					"expiretime_key2": {Value: "value2", ExpireAt: mockClock.Now().Add(4096 * time.Millisecond)},
				},
				expiretimeFunc: server.PExpireTime,
				want:           int(mockClock.Now().Add(4096 * time.Millisecond).UnixMilli()),
				wantErr:        false,
			},
			{
				name: "If the key is non-volatile, return -1",
				key:  "expiretime_key3",
				presetValues: map[string]internal.KeyData{
					"expiretime_key3": {Value: "value3", ExpireAt: time.Time{}},
				},
				expiretimeFunc: server.PExpireTime,
				want:           -1,
				wantErr:        false,
			},
			{
				name:           "If the key is non-existent return -2",
				presetValues:   nil,
				expiretimeFunc: server.PExpireTime,
				want:           -2,
				wantErr:        false,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if tt.presetValues != nil {
					for k, d := range tt.presetValues {
						presetKeyData(server, context.Background(), k, d)
					}
				}
				got, err := tt.expiretimeFunc(tt.key)
				if (err != nil) != tt.wantErr {
					t.Errorf("(P)EXPIRETIME() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if got != tt.want {
					t.Errorf("(P)EXPIRETIME() got = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("TestSugarDB_GET", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name        string
			presetValue interface{}
			key         string
			want        string
			wantErr     bool
		}{
			{
				name:        "Return string from existing key",
				presetValue: "value1",
				key:         "get_key1",
				want:        "value1",
				wantErr:     false,
			},
			{
				name:        "Return empty string if the key does not exist",
				presetValue: nil,
				key:         "get_key2",
				want:        "",
				wantErr:     false,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if tt.presetValue != nil {
					err := presetValue(server, context.Background(), tt.key, tt.presetValue)
					if err != nil {
						t.Error(err)
						return
					}
				}
				got, err := server.Get(tt.key)
				if (err != nil) != tt.wantErr {
					t.Errorf("GET() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if got != tt.want {
					t.Errorf("GET() got = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("TestSugarDB_MGET", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name         string
			presetValues map[string]interface{}
			keys         []string
			want         []string
			wantErr      bool
		}{
			{
				name: "1. Get all values in the same order the keys were provided in",
				presetValues: map[string]interface{}{
					"mget_key1": "value1", "mget_key2": "value2", "mget_key3": "value3", "mget_key4": "value4",
				},
				keys:    []string{"mget_key1", "mget_key4", "mget_key2", "mget_key3", "mget_key1"},
				want:    []string{"value1", "value4", "value2", "value3", "value1"},
				wantErr: false,
			},
			{
				name: "2. Return empty strings for non-existent keys",
				presetValues: map[string]interface{}{
					"mget_key5": "value5", "mget_key6": "value6", "mget_key7": "value7",
				},
				keys: []string{
					"mget_key5", "mget_key6", "mget_non-existent", "mget_non-existent", "mget_key7", "mget_non-existent",
				},
				want:    []string{"value5", "value6", "", "", "value7", ""},
				wantErr: false,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if tt.presetValues != nil {
					for k, v := range tt.presetValues {
						err := presetValue(server, context.Background(), k, v)
						if err != nil {
							t.Error(err)
							return
						}
					}
				}
				got, err := server.MGet(tt.keys...)
				if (err != nil) != tt.wantErr {
					t.Errorf("MGET() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if len(got) != len(tt.want) {
					t.Errorf("MGET() got = %v, want %v", got, tt.want)
				}
				for _, g := range got {
					if !slices.Contains(tt.want, g) {
						t.Errorf("MGET() got = %v, want %v", got, tt.want)
					}
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("MGET() got = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("TestSugarDB_SET", func(t *testing.T) {
		t.Parallel()

		SetOptions := func(W SetWriteOption, EX SetExOption, EXTIME int, GET bool) SETOptions {
			return SETOptions{
				WriteOpt:   W,
				ExpireOpt:  EX,
				ExpireTime: EXTIME,
				Get:        GET,
			}
		}

		tests := []struct {
			name         string
			presetValues map[string]internal.KeyData
			key          string
			value        string
			options      SETOptions
			wantOk       bool
			wantPrev     string
			wantErr      bool
		}{
			{
				name:         "1. Set normal value",
				presetValues: nil,
				key:          "set_key1",
				value:        "value1",
				options:      SetOptions(nil, nil, 0, false),
				wantOk:       true,
				wantPrev:     "",
				wantErr:      false,
			},
			{
				name:         "2. Only set the value if the key does not exist",
				presetValues: nil,
				key:          "set_key2",
				value:        "value2",
				options:      SetOptions(SETNX, nil, 0, false),
				wantOk:       true,
				wantPrev:     "",
				wantErr:      false,
			},
			{
				name: "3. Throw error when value already exists with NX flag passed",
				presetValues: map[string]internal.KeyData{
					"set_key3": {
						Value:    "preset-value3",
						ExpireAt: time.Time{},
					},
				},
				key:      "set_key3",
				value:    "value3",
				options:  SetOptions(SETNX, nil, 0, false),
				wantOk:   false,
				wantPrev: "",
				wantErr:  true,
			},
			{
				name: "4. Set new key value when key exists with XX flag passed",
				presetValues: map[string]internal.KeyData{
					"set_key4": {
						Value:    "preset-value4",
						ExpireAt: time.Time{},
					},
				},
				key:      "set_key4",
				value:    "value4",
				options:  SetOptions(SETXX, nil, 0, false),
				wantOk:   true,
				wantPrev: "",
				wantErr:  false,
			},
			{
				name:         "5. Return error when setting non-existent key with XX flag",
				presetValues: nil,
				key:          "set_key5",
				value:        "value5",
				options:      SetOptions(SETXX, nil, 0, false),
				wantOk:       false,
				wantPrev:     "",
				wantErr:      true,
			},
			{
				name:         "6. Set expiry time on the key to 100 seconds from now",
				presetValues: nil,
				key:          "set_key6",
				value:        "value6",
				options:      SetOptions(nil, SETEX, 100, false),
				wantOk:       true,
				wantPrev:     "",
				wantErr:      false,
			},
			{
				name:         "7. Set expiry time on the key in unix milliseconds",
				presetValues: nil,
				key:          "set_key7",
				value:        "value7",
				options:      SetOptions(nil, SETPX, 4096, false),
				wantOk:       true,
				wantPrev:     "",
				wantErr:      false,
			},
			{
				name:         "8. Set exact expiry time in seconds from unix epoch",
				presetValues: nil,
				key:          "set_key8",
				value:        "value8",
				options:      SetOptions(nil, SETEXAT, int(mockClock.Now().Add(200*time.Second).Unix()), false),
				wantOk:       true,
				wantPrev:     "",
				wantErr:      false,
			},
			{
				name:         "9. Set exact expiry time in milliseconds from unix epoch",
				key:          "set_key9",
				value:        "value9",
				options:      SetOptions(nil, SETPXAT, int(mockClock.Now().Add(4096*time.Millisecond).UnixMilli()), false),
				presetValues: nil,
				wantOk:       true,
				wantPrev:     "",
				wantErr:      false,
			},
			{
				name: "10. Get the previous value when GET flag is passed",
				presetValues: map[string]internal.KeyData{
					"set_key10": {
						Value:    "previous-value",
						ExpireAt: time.Time{},
					},
				},
				key:      "set_key10",
				value:    "value10",
				options:  SetOptions(nil, SETEX, 1000, true),
				wantOk:   true,
				wantPrev: "previous-value",
				wantErr:  false,
			},
			{
				name:         "11. Return nil when GET value is passed and no previous value exists",
				presetValues: nil,
				key:          "set_key11",
				value:        "value11",
				options:      SetOptions(nil, SETEX, 1000, true),
				wantOk:       true,
				wantPrev:     "",
				wantErr:      false,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if tt.presetValues != nil {
					for k, d := range tt.presetValues {
						presetKeyData(server, context.Background(), k, d)
					}
				}
				previousValue, ok, err := server.Set(
					tt.key,
					tt.value,
					tt.options,
				)
				if (err != nil) != tt.wantErr {
					t.Errorf("SET() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if ok != tt.wantOk {
					t.Errorf("SET() ok got = %v, want %v", ok, tt.wantOk)
				}
				if previousValue != tt.wantPrev {
					t.Errorf("SET() previous value got = %v, want %v", previousValue, tt.wantPrev)
				}
			})
		}
	})

	t.Run("TestSugarDB_MSET", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			kvPairs map[string]string
			want    bool
			wantErr bool
		}{
			{
				name:    "1. Set multiple keys",
				kvPairs: map[string]string{"mset_key1": "value1", "mset_key2": "10", "mset_key3": "3.142"},
				want:    true,
				wantErr: false,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got, err := server.MSet(tt.kvPairs)
				if (err != nil) != tt.wantErr {
					t.Errorf("MSET() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if got != tt.want {
					t.Errorf("MSET() got = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("TestSugarDB_PERSIST", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name         string
			presetValues map[string]internal.KeyData
			key          string
			want         bool
			wantErr      bool
		}{
			{
				name: "1. Successfully persist a volatile key",
				key:  "persist_key1",
				presetValues: map[string]internal.KeyData{
					"persist_key1": {Value: "value1", ExpireAt: mockClock.Now().Add(1000 * time.Second)},
				},
				want:    true,
				wantErr: false,
			},
			{
				name:         "2. Return false when trying to persist a non-existent key",
				key:          "persist_key2",
				presetValues: nil,
				want:         false,
				wantErr:      false,
			},
			{
				name: "3. Return false when trying to persist a non-volatile key",
				key:  "persist_key3",
				presetValues: map[string]internal.KeyData{
					"persist_key3": {Value: "value3", ExpireAt: time.Time{}},
				},
				want:    false,
				wantErr: false,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if tt.presetValues != nil {
					for k, d := range tt.presetValues {
						presetKeyData(server, context.Background(), k, d)
					}
				}
				got, err := server.Persist(tt.key)
				if (err != nil) != tt.wantErr {
					t.Errorf("PERSIST() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if got != tt.want {
					t.Errorf("PERSIST() got = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("TestSugarDB_TTL", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name         string
			presetValues map[string]internal.KeyData
			key          string
			ttlFunc      func(key string) (int, error)
			want         int
			wantErr      bool
		}{
			{
				name: "1. Return TTL time in seconds",
				key:  "ttl_key1",
				presetValues: map[string]internal.KeyData{
					"ttl_key1": {Value: "value1", ExpireAt: mockClock.Now().Add(100 * time.Second)},
				},
				ttlFunc: server.TTL,
				want:    100,
				wantErr: false,
			},
			{
				name:    "2. Return TTL time in milliseconds",
				key:     "ttl_key2",
				ttlFunc: server.PTTL,
				presetValues: map[string]internal.KeyData{
					"ttl_key2": {Value: "value2", ExpireAt: mockClock.Now().Add(4096 * time.Millisecond)},
				},
				want:    4096,
				wantErr: false,
			},
			{
				name:    "3. If the key is non-volatile, return -1",
				key:     "ttl_key3",
				ttlFunc: server.TTL,
				presetValues: map[string]internal.KeyData{
					"ttl_key3": {Value: "value3", ExpireAt: time.Time{}},
				},
				want:    -1,
				wantErr: false,
			},
			{
				name:         "4. If the key is non-existent return -2",
				key:          "ttl_key4",
				ttlFunc:      server.TTL,
				presetValues: nil,
				want:         -2,
				wantErr:      false,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if tt.presetValues != nil {
					for k, d := range tt.presetValues {
						presetKeyData(server, context.Background(), k, d)
					}
				}
				got, err := tt.ttlFunc(tt.key)
				if (err != nil) != tt.wantErr {
					t.Errorf("TTL() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if got != tt.want {
					t.Errorf("TTL() got = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("TestSugarDB_INCR", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name         string
			key          string
			presetValues map[string]internal.KeyData
			want         int
			wantErr      bool
		}{
			{
				name:         "1. Increment non-existent key",
				key:          "IncrKey1",
				presetValues: nil,
				want:         1,
				wantErr:      false,
			},
			{
				name: "2. Increment existing key with integer value",
				key:  "IncrKey2",
				presetValues: map[string]internal.KeyData{
					"IncrKey2": {Value: "5"},
				},
				want:    6,
				wantErr: false,
			},
			{
				name: "3. Increment existing key with non-integer value",
				key:  "IncrKey3",
				presetValues: map[string]internal.KeyData{
					"IncrKey3": {Value: "not_an_int"},
				},
				want:    0,
				wantErr: true,
			},
			{
				name: "4. Increment existing key with int64 value",
				key:  "IncrKey4",
				presetValues: map[string]internal.KeyData{
					"IncrKey4": {Value: int64(10)},
				},
				want:    11,
				wantErr: false,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if tt.presetValues != nil {
					for k, d := range tt.presetValues {
						presetKeyData(server, context.Background(), k, d)
					}
				}
				got, err := server.Incr(tt.key)
				if (err != nil) != tt.wantErr {
					t.Errorf("INCR() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if got != tt.want {
					t.Errorf("INCR() got = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("TestSugarDB_DECR", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name         string
			key          string
			presetValues map[string]internal.KeyData
			want         int
			wantErr      bool
		}{
			{
				name:         "1. Decrement non-existent key",
				key:          "DecrKey1",
				presetValues: nil,
				want:         -1,
				wantErr:      false,
			},
			{
				name: "2. Decrement existing key with integer value",
				key:  "DecrKey2",
				presetValues: map[string]internal.KeyData{
					"DecrKey2": {Value: "5"},
				},
				want:    4,
				wantErr: false,
			},
			{
				name: "3. Decrement existing key with non-integer value",
				key:  "DecrKey3",
				presetValues: map[string]internal.KeyData{
					"DecrKey3": {Value: "not_an_int"},
				},
				want:    0,
				wantErr: true,
			},
			{
				name: "4. Decrement existing key with int64 value",
				key:  "DecrKey4",
				presetValues: map[string]internal.KeyData{
					"DecrKey4": {Value: int64(10)},
				},
				want:    9,
				wantErr: false,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if tt.presetValues != nil {
					for k, d := range tt.presetValues {
						presetKeyData(server, context.Background(), k, d)
					}
				}
				got, err := server.Decr(tt.key)
				if (err != nil) != tt.wantErr {
					t.Errorf("DECR() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if got != tt.want {
					t.Errorf("DECR() got = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("TestSugarDB_INCRBY", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name         string
			key          string
			increment    string
			presetValues map[string]internal.KeyData
			want         int
			wantErr      bool
		}{
			{
				name:         "1. Increment non-existent key by 4",
				key:          "IncrByKey1",
				increment:    "4",
				presetValues: nil,
				want:         4,
				wantErr:      false,
			},
			{
				name:      "2. Increment existing key with integer value by 3",
				key:       "IncrByKey2",
				increment: "3",
				presetValues: map[string]internal.KeyData{
					"IncrByKey2": {Value: "5"},
				},
				want:    8,
				wantErr: false,
			},
			{
				name:      "3. Increment existing key with non-integer value by 2",
				key:       "IncrByKey3",
				increment: "2",
				presetValues: map[string]internal.KeyData{
					"IncrByKey3": {Value: "not_an_int"},
				},
				want:    0,
				wantErr: true,
			},
			{
				name:      "4. Increment existing key with int64 value by 7",
				key:       "IncrByKey4",
				increment: "7",
				presetValues: map[string]internal.KeyData{
					"IncrByKey4": {Value: int64(10)},
				},
				want:    17,
				wantErr: false,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if tt.presetValues != nil {
					for k, d := range tt.presetValues {
						presetKeyData(server, context.Background(), k, d)
					}
				}
				got, err := server.IncrBy(tt.key, tt.increment)
				if (err != nil) != tt.wantErr {
					t.Errorf("IncrBy() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if got != tt.want {
					t.Errorf("IncrBy() got = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("TestSugarDB_INCRBYFLOAT", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name         string
			key          string
			increment    string
			presetValues map[string]internal.KeyData
			want         float64
			wantErr      bool
		}{
			{
				name:         "1. Increment non-existent key by 2.5",
				key:          "IncrByFloatKey1",
				increment:    "2.5",
				presetValues: nil,
				want:         2.5,
				wantErr:      false,
			},
			{
				name:      "2. Increment existing key with integer value by 1.2",
				key:       "IncrByFloatKey2",
				increment: "1.2",
				presetValues: map[string]internal.KeyData{
					"IncrByFloatKey2": {Value: "5"},
				},
				want:    6.2,
				wantErr: false,
			},
			{
				name:      "3. Increment existing key with float value by 0.7",
				key:       "IncrByFloatKey4",
				increment: "0.7",
				presetValues: map[string]internal.KeyData{
					"IncrByFloatKey4": {Value: "10.0"},
				},
				want:    10.7,
				wantErr: false,
			},
			{
				name:      "4. Increment existing key with scientific notation value by 200",
				key:       "IncrByFloatKey5",
				increment: "200",
				presetValues: map[string]internal.KeyData{
					"IncrByFloatKey5": {Value: "5.0e3"},
				},
				want:    5200,
				wantErr: false,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if tt.presetValues != nil {
					for k, d := range tt.presetValues {
						presetKeyData(server, context.Background(), k, d)
					}
				}
				got, err := server.IncrByFloat(tt.key, tt.increment)
				if (err != nil) != tt.wantErr {
					t.Errorf("IncrByFloat() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if err == nil && got != tt.want {
					t.Errorf("IncrByFloat() got = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("TestSugarDB_DECRBY", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name         string
			key          string
			decrement    string
			presetValues map[string]internal.KeyData
			want         int
			wantErr      bool
		}{
			{
				name:         "1. Decrement non-existent key by 4",
				key:          "DecrByKey1",
				decrement:    "4",
				presetValues: nil,
				want:         -4,
				wantErr:      false,
			},
			{
				name:      "2. Decrement existing key with integer value by 3",
				key:       "DecrByKey2",
				decrement: "3",
				presetValues: map[string]internal.KeyData{
					"DecrByKey2": {Value: "-5"},
				},
				want:    -8,
				wantErr: false,
			},
			{
				name:      "3. Decrement existing key with non-integer value by 2",
				key:       "DecrByKey3",
				decrement: "2",
				presetValues: map[string]internal.KeyData{
					"DecrByKey3": {Value: "not_an_int"},
				},
				want:    0,
				wantErr: true,
			},
			{
				name:      "4. Decrement existing key with int64 value by 7",
				key:       "DecrByKey4",
				decrement: "7",
				presetValues: map[string]internal.KeyData{
					"DecrByKey4": {Value: int64(10)}},
				want:    3,
				wantErr: false,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if tt.presetValues != nil {
					for k, d := range tt.presetValues {
						presetKeyData(server, context.Background(), k, d)
					}
				}
				got, err := server.DecrBy(tt.key, tt.decrement)
				if (err != nil) != tt.wantErr {
					t.Errorf("DecrBy() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if got != tt.want {
					t.Errorf("DecrBy() got = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("TestSugarDB_Rename", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name         string
			oldKey       string
			newKey       string
			presetValues map[string]internal.KeyData
			want         string
			wantErr      bool
		}{
			{
				name:         "1. Rename existing key",
				oldKey:       "rename_oldKey1",
				newKey:       "rename_newKey1",
				presetValues: map[string]internal.KeyData{"rename_oldKey1": {Value: "value1"}},
				want:         "OK",
				wantErr:      false,
			},
			{
				name:         "2. Rename non-existent key",
				oldKey:       "rename_oldKey2",
				newKey:       "rename_newKey2",
				presetValues: nil,
				want:         "",
				wantErr:      true,
			},
			{
				name:   "3. Rename to existing key",
				oldKey: "rename_oldKey3",
				newKey: "rename_newKey4",
				presetValues: map[string]internal.KeyData{
					"rename_oldKey3": {Value: "value3"},
					"rename_newKey4": {Value: "value4"},
				},
				want:    "OK",
				wantErr: false,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if tt.presetValues != nil {
					for k, d := range tt.presetValues {
						presetKeyData(server, context.Background(), k, d)
					}
				}
				got, err := server.Rename(tt.oldKey, tt.newKey)
				if (err != nil) != tt.wantErr {
					t.Errorf("Rename() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if got != tt.want {
					t.Errorf("Rename() got = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("TestSugarDB_RENAMENX", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name         string
			oldKey       string
			newKey       string
			presetValues map[string]internal.KeyData
			want         string
			wantErr      bool
		}{
			{
				name:         "1. Rename existing key",
				oldKey:       "renamenx_oldKey1",
				newKey:       "renamenx_newKey1",
				presetValues: map[string]internal.KeyData{"renamenx_oldKey1": {Value: "value1"}},
				want:         "OK",
				wantErr:      false,
			},
			{
				name:         "2. Rename non-existent key",
				oldKey:       "renamenx_oldKey2",
				newKey:       "renamenx_newKey2",
				presetValues: nil,
				want:         "",
				wantErr:      true,
			},
			{
				name:   "3. Rename to existing key",
				oldKey: "renamenx_oldKey3",
				newKey: "renamenx_newKey4",
				presetValues: map[string]internal.KeyData{
					"renamenx_oldKey3": {Value: "value3"},
					"renamenx_newKey4": {Value: "value4"},
				},
				want:    "",
				wantErr: true,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if tt.presetValues != nil {
					for k, d := range tt.presetValues {
						presetKeyData(server, context.Background(), k, d)
					}
				}
				got, err := server.RenameNX(tt.oldKey, tt.newKey)
				if (err != nil) != tt.wantErr {
					t.Errorf("Rename() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if got != tt.want {
					t.Errorf("Rename() got = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("TestSugarDB_RANDOMKEY", func(t *testing.T) {
		t.Parallel()

		server := createSugarDB()
		t.Cleanup(func() {
			server.ShutDown()
		})

		// test without keys
		got, err := server.RandomKey()
		if err != nil {
			t.Error(err)
			return
		}
		if got != "" {
			t.Errorf("RANDOMKEY error, expected emtpy string (%v), got (%v)", []byte(""), []byte(got))
		}

		// test with keys
		testKeys := []string{"randomkey_key1", "randomkey_key2", "randomkey_key3"}
		for _, k := range testKeys {
			err := presetValue(server, context.Background(), k, "")
			if err != nil {
				t.Error(err)
				return
			}
		}

		actual, err := server.RandomKey()
		if err != nil {
			t.Error(err)
			return
		}
		if !strings.Contains(actual, "key") {
			t.Errorf("RANDOMKEY error, expected one of %v, got %s", testKeys, got)
		}

	})

	t.Run("TestSugarDB_EXISTS", func(t *testing.T) {
		t.Parallel()

		// Test with no keys
		keys := []string{"exists_key1", "exists_key2", "exists_key3"}
		existsCount, err := server.Exists(keys...)
		if err != nil {
			t.Error(err)
			return
		}
		if existsCount != 0 {
			t.Errorf("EXISTS error, expected 0, got %d", existsCount)
		}

		// Test with some keys
		for _, k := range keys {
			err := presetValue(server, context.Background(), k, "")
			if err != nil {
				t.Error(err)
				return
			}
		}

		existsCount, err = server.Exists(keys...)
		if err != nil {
			t.Error(err)
			return
		}
		if existsCount != len(keys) {
			t.Errorf("EXISTS error, expected %d, got %d", len(keys), existsCount)
		}
	})

	t.Run("TestSugarDB_DBSIZE", func(t *testing.T) {
		t.Parallel()

		server := createSugarDB()
		t.Cleanup(func() {
			server.ShutDown()
		})

		got, err := server.DBSize()
		if err != nil {
			t.Error(err)
			return
		}
		if got != 0 {
			t.Errorf("DBSIZE error, expected 0, got %d", got)
		}

		// test with keys
		testKeys := []string{"1", "2", "3"}
		for _, k := range testKeys {
			err := presetValue(server, context.Background(), k, "")
			if err != nil {
				t.Error(err)
				return
			}
		}

		got, err = server.DBSize()
		if err != nil {
			t.Error(err)
			return
		}
		if got != len(testKeys) {
			t.Errorf("DBSIZE error, expected %d, got %d", len(testKeys), got)
		}
	})

	t.Run("TestSugarDB_GETDEL", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name        string
			presetValue interface{}
			key         string
			want        string
			wantErr     bool
		}{
			{
				name:        "1. Return string from existing key",
				presetValue: "value1",
				key:         "getdel_key1",
				want:        "value1",
				wantErr:     false,
			},
			{
				name:        "2. Return empty string if the key does not exist",
				presetValue: nil,
				key:         "getdel_key2",
				want:        "",
				wantErr:     false,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if tt.presetValue != nil {
					err := presetValue(server, context.Background(), tt.key, tt.presetValue)
					if err != nil {
						t.Error(err)
						return
					}
				}
				// Check value received
				got, err := server.GetDel(tt.key)
				if (err != nil) != tt.wantErr {
					t.Errorf("GETDEL() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if got != tt.want {
					t.Errorf("GETDEL() got = %v, want %v", got, tt.want)
				}
				// Check key was deleted
				if tt.presetValue != nil {
					got, err := server.Get(tt.key)
					if (err != nil) != tt.wantErr {
						t.Errorf("GETDEL() error = %v, wantErr %v", err, tt.wantErr)
						return
					}
					if got != "" {
						t.Errorf("GETDEL() got = %v, want empty string", got)
					}
				}
			})
		}
	})

	t.Run("TestSugarDB_GETEX", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name         string
			presetValue  interface{}
			getExOpt     GetExOption
			getExOptTime int
			key          string
			want         string
			wantEx       int
			wantErr      bool
		}{
			{
				name:        "1. Return string from existing key, no expire options",
				presetValue: "value1",
				getExOpt:    nil,
				key:         "getex_key1",
				want:        "value1",
				wantEx:      -1,
				wantErr:     false,
			},
			{
				name:         "2. Return empty string if the key does not exist",
				presetValue:  nil,
				getExOpt:     EX,
				getExOptTime: int(mockClock.Now().Add(100 * time.Second).Unix()),
				key:          "getex_key2",
				want:         "",
				wantEx:       0,
				wantErr:      false,
			},
			{
				name:         "3. Return key set expiry with EX",
				presetValue:  "value3",
				getExOpt:     EX,
				getExOptTime: 100,
				key:          "getex_key3",
				want:         "value3",
				wantEx:       100,
				wantErr:      false,
			},
			{
				name:         "4. Return key set expiry with PX",
				presetValue:  "value4",
				getExOpt:     PX,
				getExOptTime: 100000,
				key:          "getex_key4",
				want:         "value4",
				wantEx:       100,
				wantErr:      false,
			},
			{
				name:         "5. Return key set expiry with EXAT",
				presetValue:  "value5",
				getExOpt:     EXAT,
				getExOptTime: int(mockClock.Now().Add(100 * time.Second).Unix()),
				key:          "getex_key5",
				want:         "value5",
				wantEx:       100,
				wantErr:      false,
			},
			{
				name:         "6. Return key set expiry with PXAT",
				presetValue:  "value6",
				getExOpt:     PXAT,
				getExOptTime: int(mockClock.Now().Add(100 * time.Second).UnixMilli()),
				key:          "getex_key6",
				want:         "value6",
				wantEx:       100,
				wantErr:      false,
			},
			{
				name:        "7. Return key passing PERSIST",
				presetValue: "value7",
				getExOpt:    PERSIST,
				key:         "getex_key7",
				want:        "value7",
				wantEx:      -1,
				wantErr:     false,
			},
			{
				name:         "8. Return key passing PERSIST, and include a UNIXTIME",
				presetValue:  "value8",
				getExOpt:     PERSIST,
				getExOptTime: int(mockClock.Now().Add(100 * time.Second).Unix()),
				key:          "getex_key8",
				want:         "value8",
				wantEx:       -1,
				wantErr:      false,
			},
			{
				name:        "9. Return key and attempt to set expiry with EX without providing UNIXTIME",
				presetValue: "value9",
				getExOpt:    EX,
				key:         "getex_key9",
				want:        "value9",
				wantEx:      -1,
				wantErr:     false,
			},
			{
				name:        "10. Return key and attempt to set expiry with PXAT without providing UNIXTIME",
				presetValue: "value10",
				getExOpt:    PXAT,
				key:         "getex_key10",
				want:        "value10",
				wantEx:      -1,
				wantErr:     false,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if tt.presetValue != nil {
					err := presetValue(server, context.Background(), tt.key, tt.presetValue)
					if err != nil {
						t.Error(err)
						return
					}
				}
				// Check value received
				got, err := server.GetEx(tt.key, tt.getExOpt, tt.getExOptTime)
				if (err != nil) != tt.wantErr {
					t.Errorf("GETEX() GET error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if got != tt.want {
					t.Errorf("GETEX() GET - got = %v, want %v", got, tt.want)
				}
				// Check expiry was set
				if tt.presetValue != nil {
					actual, err := server.TTL(tt.key)
					if (err != nil) != tt.wantErr {
						t.Errorf("GETEX() EXPIRY error = %v, wantErr %v", err, tt.wantErr)
						return
					}
					if actual != tt.wantEx {
						t.Errorf("GETEX() EXPIRY - got = %v, want %v", actual, tt.wantEx)
					}
				}
			})
		}
	})

	// Tests Touch and OBJECTFREQ commands
	t.Run("TestSugarDB_LFU_TOUCH", func(t *testing.T) {
		t.Parallel()

		duration := time.Duration(30) * time.Second

		server := createSugarDBWithConfig(config.Config{
			DataDir:          "",
			EvictionPolicy:   constants.AllKeysLFU,
			EvictionInterval: duration,
			MaxMemory:        4000000,
		})
		t.Cleanup(func() {
			server.ShutDown()
		})

		tests := []struct {
			name     string
			keys     []string
			setKeys  []bool
			want     int
			wantErrs []bool
		}{
			{
				name:     "1. Touch key that exists.",
				keys:     []string{"Key1"},
				setKeys:  []bool{true},
				want:     1,
				wantErrs: []bool{false},
			},
			{
				name:     "2. Touch key that doesn't exist.",
				keys:     []string{"Key2"},
				setKeys:  []bool{false},
				want:     0,
				wantErrs: []bool{true},
			},
			{
				name:     "3. Touch multiple keys that all exist.",
				keys:     []string{"Key3", "Key3.1"},
				setKeys:  []bool{true, true},
				want:     2,
				wantErrs: []bool{false, false},
			},
			{
				name:     "4. Touch multiple keys, some don't exist.",
				keys:     []string{"Key4", "Key4.9"},
				setKeys:  []bool{true, false},
				want:     1,
				wantErrs: []bool{false, true},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Preset values
				for i, key := range tt.keys {
					if tt.setKeys[i] {
						err := presetValue(server, context.Background(), key, "___")
						if err != nil {
							t.Error(err)
							return
						}
					}
				}

				// Touch keys
				got, err := server.Touch(tt.keys...)
				if err != nil {
					t.Errorf("TOUCH() error - %v", err)
				}

				if got != tt.want {
					t.Errorf("TOUCH() got %v, want %v, using keys %v setKeys %v", got, tt.want, tt.keys, tt.setKeys)
				}

				// Another touch to help testing object freq
				got, err = server.Touch(tt.keys...)
				if err != nil {
					t.Errorf("TOUCH() error - %v", err)
				}

				if got != tt.want {
					t.Errorf("TOUCH() got %v, want %v, using keys %v setKeys %v", got, tt.want, tt.keys, tt.setKeys)
				}

				// Wait to avoid race
				ticker := time.NewTicker(300 * time.Millisecond)
				<-ticker.C
				ticker.Stop()

				// Objectfreq
				for i, key := range tt.keys {
					actual, err := server.ObjectFreq(key)
					if (err != nil) != tt.wantErrs[i] {
						t.Errorf("OBJECTFREQ() error: %v, wanted error: %v", err, tt.wantErrs[i])
					}
					if !tt.wantErrs[i] && actual != 3 {
						t.Errorf("OBJECTFREQ() error - expected 3 got %v for key %v", actual, key)
					}

					// Check error for object idletime
					_, err = server.ObjectIdleTime(key)
					if err == nil {
						t.Errorf("OBJECTIDLETIME() error - expected error when used on server with lfu eviction policy but got none.")
					}

				}

			})
		}
	})

	// Tests Touch and OBJECTIDLETIME commands
	t.Run("TestSugarDB_LRU_TOUCH", func(t *testing.T) {
		t.Parallel()

		duration := time.Duration(30) * time.Second

		server := createSugarDBWithConfig(config.Config{
			DataDir:          "",
			EvictionPolicy:   constants.AllKeysLRU,
			EvictionInterval: duration,
			MaxMemory:        4000000,
		})
		t.Cleanup(func() {
			server.ShutDown()
		})

		tests := []struct {
			name     string
			keys     []string
			setKeys  []bool
			want     int
			wantErrs []bool
		}{
			{
				name:     "1. Touch key that exists.",
				keys:     []string{"Key1"},
				setKeys:  []bool{true},
				want:     1,
				wantErrs: []bool{false},
			},
			{
				name:     "2. Touch key that doesn't exist.",
				keys:     []string{"Key2"},
				setKeys:  []bool{false},
				want:     0,
				wantErrs: []bool{true},
			},
			{
				name:     "3. Touch multiple keys that all exist.",
				keys:     []string{"Key3", "Key3.1"},
				setKeys:  []bool{true, true},
				want:     2,
				wantErrs: []bool{false, false},
			},
			{
				name:     "4. Touch multiple keys, some don't exist.",
				keys:     []string{"Key4", "Key4.9"},
				setKeys:  []bool{true, false},
				want:     1,
				wantErrs: []bool{false, true},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Preset values
				for i, key := range tt.keys {
					if tt.setKeys[i] {
						err := presetValue(server, context.Background(), key, "___")
						if err != nil {
							t.Error(err)
							return
						}
					}
				}

				// Touch keys
				got, err := server.Touch(tt.keys...)
				if err != nil {
					t.Errorf("TOUCH() error - %v", err)
				}

				if got != tt.want {
					t.Errorf("TOUCH() got %v, want %v, using keys %v setKeys %v", got, tt.want, tt.keys, tt.setKeys)
				}

				// Sleep to more easily test Object idle time
				// TODO: Update this ticker when updateKeysInCache implementation is updated
				// Due to the event-based command execution, the actual touch may be done slightly later
				// than the invocation time as it waits for earlier events to be handled.
				ticker := time.NewTicker(200 * time.Millisecond)
				<-ticker.C
				ticker.Stop()

				// Objectidletime
				for i, key := range tt.keys {
					actual, err := server.ObjectIdleTime(key)
					if (err != nil) != tt.wantErrs[i] {
						t.Errorf("OBJECTIDLETIME() error: %v, wanted error: %v", err, tt.wantErrs[i])
					}
					if !tt.wantErrs[i] && (actual <= 0) { // TODO: Fix updated condition to account for touch delay
						t.Errorf("OBJECTIDLETIME() error - expected 0.2 got %v", actual)
					}

					// Check error for object freq
					_, err = server.ObjectFreq(key)
					if err == nil {
						t.Errorf("OBJECTFREQ() error - expected error when used on server with lru eviction policy but got none.")
					}
				}

			})
		}
	})

	t.Run("TestSugarDB_TYPE", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name        string
			presetValue interface{}
			key         string
			want        string
			wantErr     bool
		}{
			{
				name:        "1. Return string from existing key",
				presetValue: "value1",
				key:         "type_key1",
				want:        "string",
				wantErr:     false,
			},
			{
				name:        "2. Return empty string if the key does not exist",
				presetValue: nil,
				key:         "type_key2",
				want:        "",
				wantErr:     true,
			},
			{
				name:        "3. Return string from existing key",
				presetValue: 10,
				key:         "type_key3",
				want:        "integer",
				wantErr:     false,
			},
			{
				name:        "4. Return string from existing key",
				presetValue: 10.1,
				key:         "type_key4",
				want:        "float",
				wantErr:     false,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if tt.presetValue != nil {
					err := presetValue(server, context.Background(), tt.key, tt.presetValue)
					if err != nil {
						t.Error(err)
						return
					}
				}
				got, err := server.Type(tt.key)
				if (err != nil) != tt.wantErr {
					t.Errorf("GET() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if got != tt.want {
					t.Errorf("GET() got = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("TestSugarDB_COPY", func(t *testing.T) {
		t.Parallel()

		CopyOptions := func(DB string, R bool) COPYOptions {
			return COPYOptions{
				Database: DB,
				Replace:  R,
			}
		}

		tests := []struct {
			name                 string
			sourceKeyPresetValue interface{}
			sourceKey            string
			destKeyPresetValue   interface{}
			destinationKey       string
			options              COPYOptions
			expectedValue        string
			want                 int
			wantErr              bool
		}{
			{
				name:                 "1. Copy Value into non existing key",
				sourceKeyPresetValue: "value1",
				sourceKey:            "copy_skey1",
				destKeyPresetValue:   nil,
				destinationKey:       "copy_dkey1",
				options:              CopyOptions("0", false),
				expectedValue:        "value1",
				want:                 1,
				wantErr:              false,
			},
			{
				name:                 "2. Copy Value into existing key without replace option",
				sourceKeyPresetValue: "value2",
				sourceKey:            "copy_skey2",
				destKeyPresetValue:   "dValue2",
				destinationKey:       "copy_dkey2",
				options:              CopyOptions("0", false),
				expectedValue:        "dValue2",
				want:                 0,
				wantErr:              false,
			},
			{
				name:                 "3. Copy Value into existing key with replace option",
				sourceKeyPresetValue: "value3",
				sourceKey:            "copy_skey3",
				destKeyPresetValue:   "dValue3",
				destinationKey:       "copy_dkey3",
				options:              CopyOptions("0", true),
				expectedValue:        "value3",
				want:                 1,
				wantErr:              false,
			},
			{
				name:                 "4. Copy Value into different database",
				sourceKeyPresetValue: "value4",
				sourceKey:            "copy_skey4",
				destKeyPresetValue:   nil,
				destinationKey:       "copy_dkey4",
				options:              CopyOptions("1", false),
				expectedValue:        "value4",
				want:                 1,
				wantErr:              false,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if tt.sourceKeyPresetValue != nil {
					err := presetValue(server, context.Background(), tt.sourceKey, tt.sourceKeyPresetValue)
					if err != nil {
						t.Error(err)
						return
					}
				}
				if tt.destKeyPresetValue != nil {
					err := presetValue(server, context.Background(), tt.destinationKey, tt.destKeyPresetValue)
					if err != nil {
						t.Error(err)
						return
					}
				}

				got, err := server.Copy(tt.sourceKey, tt.destinationKey, tt.options)
				if (err != nil) != tt.wantErr {
					t.Errorf("COPY() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if got != tt.want {
					t.Errorf("COPY() got = %v, want %v", got, tt.want)
				}

				val, err := getValue(server, context.Background(), tt.destinationKey, tt.options.Database)
				if err != nil {
					t.Error(err)
					return
				}

				if val != tt.expectedValue {
					t.Errorf("COPY() value in destionation key: %v, should be: %v", val, tt.expectedValue)
				}
			})
		}
	})

	t.Run("TestSugarDB_MOVE", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name        string
			presetValue interface{}
			key         string
			want        int
		}{
			{
				name:        "1. Move key successfully",
				presetValue: "value1",
				key:         "move_key1",
				want:        1,
			},
			{
				name:        "2. Attempt to move key, unsuccessful",
				presetValue: nil,
				key:         "move_key2",
				want:        0,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				t.Log(tt.name)
				if tt.presetValue != nil {
					err := presetValue(server, context.Background(), tt.key, tt.presetValue)
					if err != nil {
						t.Error(err)
						return
					}
				}

				got, err := server.Move(tt.key, 1)
				if err != nil {
					t.Error(err)
				}

				if got != tt.want {
					t.Errorf("MOVE() got %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("TestSugarDB_KEYS", func(t *testing.T) {
		tests := []struct {
			name         string
			presetValues map[string]interface{}
			pattern      string
			want         []string
			wantErr      bool
		}{
			{
				name: "1. Return all keys with * pattern",
				presetValues: map[string]interface{}{
					"keys_key1": "value1",
					"keys_key2": "value2",
					"keys_key3": "value3",
				},
				pattern: "*",
				want:    []string{"keys_key1", "keys_key2", "keys_key3"},
				wantErr: false,
			},
			{
				name: "2. Return keys matching specific pattern",
				presetValues: map[string]interface{}{
					"keys_key1": "value1",
					"keys_key2": "value2",
					"other_key": "value3",
				},
				pattern: "keys_*",
				want:    []string{"keys_key1", "keys_key2"},
				wantErr: false,
			},
			{
				name: "3. Return keys matching single character pattern",
				presetValues: map[string]interface{}{
					"keys_key1": "value1",
					"keys_key2": "value2",
					"keys_kex3": "value3",
				},
				pattern: "keys_key?",
				want:    []string{"keys_key1", "keys_key2"},
				wantErr: false,
			},
			{
				name: "4. Return empty slice when no keys match pattern",
				presetValues: map[string]interface{}{
					"keys_key1": "value1",
					"keys_key2": "value2",
				},
				pattern: "nonexistent_*",
				want:    []string{},
				wantErr: false,
			},
			{
				name:         "5. Return empty slice when no keys exist",
				presetValues: nil,
				pattern:      "*",
				want:         []string{},
				wantErr:      false,
			},
			{
				name: "6. Return keys matching character class pattern",
				presetValues: map[string]interface{}{
					"keys_key1": "value1",
					"keys_key2": "value2",
					"keys_kex3": "value3",
				},
				pattern: "keys_key[12]",
				want:    []string{"keys_key1", "keys_key2"},
				wantErr: false,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Create a new server instance for each test case
				server := createSugarDB()
				t.Cleanup(func() {
					server.ShutDown()
				})

				if tt.presetValues != nil {
					for k, v := range tt.presetValues {
						err := presetValue(server, context.Background(), k, v)
						if err != nil {
							t.Error(err)
							return
						}
					}
				}
				got, err := server.Keys(tt.pattern)
				if (err != nil) != tt.wantErr {
					t.Errorf("KEYS() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				slices.Sort(got)
				slices.Sort(tt.want)
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("KEYS() got = %v, want %v", got, tt.want)
				}
			})
		}
	})
}
