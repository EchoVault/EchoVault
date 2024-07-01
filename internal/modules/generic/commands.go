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

package generic

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/echovault/echovault/internal"
	"github.com/echovault/echovault/internal/constants"
)

type KeyObject struct {
	value  interface{}
	locked bool
}

func handleSet(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := setKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	keyExists := params.KeysExist(params.Context, keys.WriteKeys)[key]
	value := params.Command[2]
	res := []byte(constants.OkResponse)
	clock := params.GetClock()

	options, err := getSetCommandOptions(clock, params.Command[3:], SetOptions{})
	if err != nil {
		return nil, err
	}

	// If Get is provided, the response should be the current stored value.
	// If there's no current value, then the response should be nil.
	if options.get {
		if !keyExists {
			res = []byte("$-1\r\n")
		} else {
			res = []byte(fmt.Sprintf("+%v\r\n", params.GetValues(params.Context, []string{key})[key]))
		}
	}

	if "xx" == strings.ToLower(options.exists) {
		// If XX is specified, make sure the key exists.
		if !keyExists {
			return nil, fmt.Errorf("key %s does not exist", key)
		}
	} else if "nx" == strings.ToLower(options.exists) {
		// If NX is specified, make sure that the key does not currently exist.
		if keyExists {
			return nil, fmt.Errorf("key %s already exists", key)
		}
	}

	if err = params.SetValues(params.Context, map[string]interface{}{
		key: internal.AdaptType(value),
	}); err != nil {
		return nil, err
	}

	// If expiresAt is set, set the key's expiry time as well
	if options.expireAt != nil {
		params.SetExpiry(params.Context, key, options.expireAt.(time.Time), false)
	}

	return res, nil
}

func handleMSet(params internal.HandlerFuncParams) ([]byte, error) {
	_, err := msetKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	entries := make(map[string]interface{})

	// Extract all the key/value pairs
	for i, key := range params.Command[1:] {
		if i%2 == 0 {
			entries[key] = internal.AdaptType(params.Command[1:][i+1])
		}
	}

	// Set all the values
	if err = params.SetValues(params.Context, entries); err != nil {
		return nil, err
	}

	return []byte(constants.OkResponse), nil
}

func handleGet(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := getKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}
	key := keys.ReadKeys[0]
	keyExists := params.KeysExist(params.Context, []string{key})[key]

	if !keyExists {
		return []byte("$-1\r\n"), nil
	}

	value := params.GetValues(params.Context, []string{key})[key]

	return []byte(fmt.Sprintf("+%v\r\n", value)), nil
}

func handleMGet(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := mgetKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	values := make(map[string]string)
	for key, value := range params.GetValues(params.Context, keys.ReadKeys) {
		if value == nil {
			values[key] = ""
			continue
		}
		values[key] = fmt.Sprintf("%v", value)
	}

	bytes := []byte(fmt.Sprintf("*%d\r\n", len(params.Command[1:])))

	for _, key := range params.Command[1:] {
		if values[key] == "" {
			bytes = append(bytes, []byte("$-1\r\n")...)
			continue
		}
		bytes = append(bytes, []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(values[key]), values[key]))...)
	}

	return bytes, nil
}

func handleDel(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := delKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}
	count := 0
	for key, exists := range params.KeysExist(params.Context, keys.WriteKeys) {
		if !exists {
			continue
		}
		err = params.DeleteKey(params.Context, key)
		if err != nil {
			log.Printf("could not delete key %s due to error: %+v\n", key, err)
			continue
		}
		count += 1
	}
	return []byte(fmt.Sprintf(":%d\r\n", count)), nil
}

func handlePersist(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := persistKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	keyExists := params.KeysExist(params.Context, keys.WriteKeys)[key]

	if !keyExists {
		return []byte(":0\r\n"), nil
	}

	expireAt := params.GetExpiry(params.Context, key)
	if expireAt == (time.Time{}) {
		return []byte(":0\r\n"), nil
	}

	params.SetExpiry(params.Context, key, time.Time{}, false)

	return []byte(":1\r\n"), nil
}

func handleExpireTime(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := expireTimeKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	keyExists := params.KeysExist(params.Context, keys.ReadKeys)[key]

	if !keyExists {
		return []byte(":-2\r\n"), nil
	}

	expireAt := params.GetExpiry(params.Context, key)

	if expireAt == (time.Time{}) {
		return []byte(":-1\r\n"), nil
	}

	t := expireAt.Unix()
	if strings.ToLower(params.Command[0]) == "pexpiretime" {
		t = expireAt.UnixMilli()
	}

	return []byte(fmt.Sprintf(":%d\r\n", t)), nil
}

func handleTTL(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := ttlKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	keyExists := params.KeysExist(params.Context, keys.ReadKeys)[key]

	clock := params.GetClock()

	if !keyExists {
		return []byte(":-2\r\n"), nil
	}

	expireAt := params.GetExpiry(params.Context, key)

	if expireAt == (time.Time{}) {
		return []byte(":-1\r\n"), nil
	}

	t := expireAt.Unix() - clock.Now().Unix()
	if strings.ToLower(params.Command[0]) == "pttl" {
		t = expireAt.UnixMilli() - clock.Now().UnixMilli()
	}

	if t <= 0 {
		return []byte(":0\r\n"), nil
	}

	return []byte(fmt.Sprintf(":%d\r\n", t)), nil
}

func handleExpire(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := expireKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	keyExists := params.KeysExist(params.Context, keys.WriteKeys)[key]

	// Extract time
	n, err := strconv.ParseInt(params.Command[2], 10, 64)
	if err != nil {
		return nil, errors.New("expire time must be integer")
	}
	expireAt := params.GetClock().Now().Add(time.Duration(n) * time.Second)
	if strings.ToLower(params.Command[0]) == "pexpire" {
		expireAt = params.GetClock().Now().Add(time.Duration(n) * time.Millisecond)
	}

	if !keyExists {
		return []byte(":0\r\n"), nil
	}

	if len(params.Command) == 3 {
		params.SetExpiry(params.Context, key, expireAt, true)
		return []byte(":1\r\n"), nil
	}

	currentExpireAt := params.GetExpiry(params.Context, key)

	switch strings.ToLower(params.Command[3]) {
	case "nx":
		if currentExpireAt != (time.Time{}) {
			return []byte(":0\r\n"), nil
		}
		params.SetExpiry(params.Context, key, expireAt, false)
	case "xx":
		if currentExpireAt == (time.Time{}) {
			return []byte(":0\r\n"), nil
		}
		params.SetExpiry(params.Context, key, expireAt, false)
	case "gt":
		if currentExpireAt == (time.Time{}) {
			return []byte(":0\r\n"), nil
		}
		if expireAt.Before(currentExpireAt) {
			return []byte(":0\r\n"), nil
		}
		params.SetExpiry(params.Context, key, expireAt, false)
	case "lt":
		if currentExpireAt != (time.Time{}) {
			if currentExpireAt.Before(expireAt) {
				return []byte(":0\r\n"), nil
			}
			params.SetExpiry(params.Context, key, expireAt, false)
		}
		params.SetExpiry(params.Context, key, expireAt, false)
	default:
		return nil, fmt.Errorf("unknown option %s", strings.ToUpper(params.Command[3]))
	}

	return []byte(":1\r\n"), nil
}

func handleExpireAt(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := expireKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	keyExists := params.KeysExist(params.Context, keys.WriteKeys)[key]

	// Extract time
	n, err := strconv.ParseInt(params.Command[2], 10, 64)
	if err != nil {
		return nil, errors.New("expire time must be integer")
	}
	expireAt := time.Unix(n, 0)
	if strings.ToLower(params.Command[0]) == "pexpireat" {
		expireAt = time.UnixMilli(n)
	}

	if !keyExists {
		return []byte(":0\r\n"), nil
	}

	if len(params.Command) == 3 {
		params.SetExpiry(params.Context, key, expireAt, true)
		return []byte(":1\r\n"), nil
	}

	currentExpireAt := params.GetExpiry(params.Context, key)

	switch strings.ToLower(params.Command[3]) {
	case "nx":
		if currentExpireAt != (time.Time{}) {
			return []byte(":0\r\n"), nil
		}
		params.SetExpiry(params.Context, key, expireAt, false)
	case "xx":
		if currentExpireAt == (time.Time{}) {
			return []byte(":0\r\n"), nil
		}
		params.SetExpiry(params.Context, key, expireAt, false)
	case "gt":
		if currentExpireAt == (time.Time{}) {
			return []byte(":0\r\n"), nil
		}
		if expireAt.Before(currentExpireAt) {
			return []byte(":0\r\n"), nil
		}
		params.SetExpiry(params.Context, key, expireAt, false)
	case "lt":
		if currentExpireAt != (time.Time{}) {
			if currentExpireAt.Before(expireAt) {
				return []byte(":0\r\n"), nil
			}
			params.SetExpiry(params.Context, key, expireAt, false)
		}
		params.SetExpiry(params.Context, key, expireAt, false)
	default:
		return nil, fmt.Errorf("unknown option %s", strings.ToUpper(params.Command[3]))
	}

	return []byte(":1\r\n"), nil
}

func handleIncr(params internal.HandlerFuncParams) ([]byte, error) {
	// Extract key from command
	keys, err := incrKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	values := params.GetValues(params.Context, []string{key}) // Get the current values for the specified keys
	currentValue, ok := values[key]                           // Check if the key exists

	var newValue int64
	var currentValueInt int64

	// Check if the key exists and its current value
	if !ok || currentValue == nil {
		// If key does not exist, initialize it with 1
		newValue = 1
	} else {
		// Use type switch to handle different types of currentValue
		switch v := currentValue.(type) {
		case string:
			var err error
			currentValueInt, err = strconv.ParseInt(v, 10, 64) // Parse the string to int64
			if err != nil {
				return nil, errors.New("value is not an integer or out of range")
			}
		case int:
			currentValueInt = int64(v) // Convert int to int64
		case int64:
			currentValueInt = v // Use int64 value directly
		default:
			fmt.Printf("unexpected type for currentValue: %T\n", currentValue)
			return nil, errors.New("unexpected type for currentValue") // Handle unexpected types
		}
		newValue = currentValueInt + 1 // Increment the value
	}

	// Set the new incremented value
	if err := params.SetValues(params.Context, map[string]interface{}{key: fmt.Sprintf("%d", newValue)}); err != nil {
		return nil, err
	}

	// Prepare response with the actual new value
	return []byte(fmt.Sprintf(":%d\r\n", newValue)), nil
}

func handleDecr(params internal.HandlerFuncParams) ([]byte, error) {
	// Extract key from command
	keys, err := decrKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	values := params.GetValues(params.Context, []string{key}) // Get the current values for the specified keys
	currentValue, ok := values[key]                           // Check if the key exists

	var newValue int64
	var currentValueInt int64

	// Check if the key exists and its current value
	if !ok || currentValue == nil {
		// If key does not exist, initialize it with 0
		newValue = -1
	} else {
		// Use type switch to handle different types of currentValue
		switch v := currentValue.(type) {
		case string:
			var err error
			currentValueInt, err = strconv.ParseInt(v, 10, 64) // Parse the string to int64
			if err != nil {
				return nil, errors.New("value is not an integer or out of range")
			}
		case int:
			currentValueInt = int64(v) // Convert int to int64
		case int64:
			currentValueInt = v // Use int64 value directly
		default:
			fmt.Printf("unexpected type for currentValue: %T\n", currentValue)
			return nil, errors.New("unexpected type for currentValue") // Handle unexpected types
		}
		newValue = currentValueInt - 1 // Decrement the value
	}

	// Set the new incremented value
	if err := params.SetValues(params.Context, map[string]interface{}{key: fmt.Sprintf("%d", newValue)}); err != nil {
		return nil, err
	}

	// Prepare response with the actual new value
	return []byte(fmt.Sprintf(":%d\r\n", newValue)), nil
}

func handleIncrBy(params internal.HandlerFuncParams) ([]byte, error) {
	// Extract key from command
	keys, err := incrByKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	// Parse increment value
	incrValue, err := strconv.ParseInt(params.Command[2], 10, 64)
	if err != nil {
		return nil, errors.New("increment value is not an integer or out of range")
	}

	key := keys.WriteKeys[0]
	values := params.GetValues(params.Context, []string{key}) // Get the current values for the specified keys
	currentValue, ok := values[key]                           // Check if the key exists

	var newValue int64
	var currentValueInt int64

	// Check if the key exists and its current value
	if !ok || currentValue == nil {
		// If key does not exist, initialize it with the increment value
		newValue = incrValue
	} else {
		// Use type switch to handle different types of currentValue
		switch v := currentValue.(type) {
		case string:
			currentValueInt, err = strconv.ParseInt(v, 10, 64) // Parse the string to int64
			if err != nil {
				return nil, errors.New("value is not an integer or out of range")
			}
		case int:
			currentValueInt = int64(v) // Convert int to int64
		case int64:
			currentValueInt = v // Use int64 value directly
		default:
			fmt.Printf("unexpected type for currentValue: %T\n", currentValue)
			return nil, errors.New("unexpected type for currentValue") // Handle unexpected types
		}
		newValue = currentValueInt + incrValue // Increment the value by the specified amount
	}

	// Set the new incremented value
	if err := params.SetValues(params.Context, map[string]interface{}{key: fmt.Sprintf("%d", newValue)}); err != nil {
		return nil, err
	}

	// Prepare response with the actual new value
	return []byte(fmt.Sprintf(":%d\r\n", newValue)), nil
}

func handleDecrBy(params internal.HandlerFuncParams) ([]byte, error) {
	// Extract key from command
	keys, err := decrByKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	// Parse decrement value
	decrValue, err := strconv.ParseInt(params.Command[2], 10, 64)
	if err != nil {
		return nil, errors.New("decrement value is not an integer or out of range")
	}

	key := keys.WriteKeys[0]
	values := params.GetValues(params.Context, []string{key}) // Get the current values for the specified keys
	currentValue, ok := values[key]                           // Check if the key exists

	var newValue int64
	var currentValueInt int64

	// Check if the key exists and its current value
	if !ok || currentValue == nil {
		// If key does not exist, initialize it with the decrement value
		newValue = decrValue * -1
	} else {
		// Use type switch to handle different types of currentValue
		switch v := currentValue.(type) {
		case string:
			currentValueInt, err = strconv.ParseInt(v, 10, 64) // Parse the string to int64
			if err != nil {
				return nil, errors.New("value is not an integer or out of range")
			}
		case int:
			currentValueInt = int64(v) // Convert int to int64
		case int64:
			currentValueInt = v // Use int64 value directly
		default:
			fmt.Printf("unexpected type for currentValue: %T\n", currentValue)
			return nil, errors.New("unexpected type for currentValue") // Handle unexpected types
		}
		newValue = currentValueInt - decrValue // decrement the value by the specified amount
	}

	// Set the new incremented value
	if err := params.SetValues(params.Context, map[string]interface{}{key: fmt.Sprintf("%d", newValue)}); err != nil {
		return nil, err
	}

	// Prepare response with the actual new value
	return []byte(fmt.Sprintf(":%d\r\n", newValue)), nil
}

func handleRename(params internal.HandlerFuncParams) ([]byte, error) {
	if len(params.Command) != 3 {
		return nil, errors.New(constants.WrongArgsResponse)
	}

	oldKey := params.Command[1]
	newKey := params.Command[2]

	// Get the current value for the old key
	values := params.GetValues(params.Context, []string{oldKey})
	oldValue, ok := values[oldKey]

	if !ok || oldValue == nil {
		return nil, errors.New("no such key")
	}

	// Set the new key with the old value
	if err := params.SetValues(params.Context, map[string]interface{}{newKey: oldValue}); err != nil {
		return nil, err
	}

	// Delete the old key
	if err := params.DeleteKey(params.Context, oldKey); err != nil {
		return nil, err
	}

	return []byte("+OK\r\n"), nil
}

func handleFlush(params internal.HandlerFuncParams) ([]byte, error) {
	if len(params.Command) != 1 {
		return nil, errors.New(constants.WrongArgsResponse)
	}

	if strings.EqualFold(params.Command[0], "flushall") {
		params.Flush(-1)
		return []byte(constants.OkResponse), nil
	}

	database := params.Context.Value("Database").(int)
	params.Flush(database)
	return []byte(constants.OkResponse), nil
}

func Commands() []internal.Command {
	return []internal.Command{
		{
			Command:    "set",
			Module:     constants.GenericModule,
			Categories: []string{constants.WriteCategory, constants.SlowCategory},
			Description: `
(SET key value [NX | XX] [GET] [EX seconds | PX milliseconds | EXAT unix-time-seconds | PXAT unix-time-milliseconds])
Set the value of a key, considering the value's type.
NX - Only set if the key does not exist.
XX - Only set if the key exists.
GET - Return the old value stored at key, or nil if the value does not exist.
EX - Expire the key after the specified number of seconds (positive integer).
PX - Expire the key after the specified number of milliseconds (positive integer).
EXAT - Expire at the exact time in unix seconds (positive integer).
PXAT - Expire at the exat time in unix milliseconds (positive integer).`,
			Sync:              true,
			KeyExtractionFunc: setKeyFunc,
			HandlerFunc:       handleSet,
		},
		{
			Command:           "mset",
			Module:            constants.GenericModule,
			Categories:        []string{constants.WriteCategory, constants.SlowCategory},
			Description:       "(MSET key value [key value ...]) Automatically set or modify multiple key/value pairs.",
			Sync:              true,
			KeyExtractionFunc: msetKeyFunc,
			HandlerFunc:       handleMSet,
		},
		{
			Command:           "get",
			Module:            constants.GenericModule,
			Categories:        []string{constants.ReadCategory, constants.FastCategory},
			Description:       "(GET key) Get the value at the specified key.",
			Sync:              false,
			KeyExtractionFunc: getKeyFunc,
			HandlerFunc:       handleGet,
		},
		{
			Command:           "mget",
			Module:            constants.GenericModule,
			Categories:        []string{constants.ReadCategory, constants.FastCategory},
			Description:       "(MGET key [key ...]) Get multiple values from the specified keys.",
			Sync:              false,
			KeyExtractionFunc: mgetKeyFunc,
			HandlerFunc:       handleMGet,
		},
		{
			Command:           "del",
			Module:            constants.GenericModule,
			Categories:        []string{constants.KeyspaceCategory, constants.WriteCategory, constants.FastCategory},
			Description:       "(DEL key [key ...]) Removes one or more keys from the store.",
			Sync:              true,
			KeyExtractionFunc: delKeyFunc,
			HandlerFunc:       handleDel,
		},
		{
			Command:    "persist",
			Module:     constants.GenericModule,
			Categories: []string{constants.KeyspaceCategory, constants.WriteCategory, constants.FastCategory},
			Description: `(PERSIST key) Removes the TTl associated with a key,
turning it from a volatile key to a persistent key.`,
			Sync:              true,
			KeyExtractionFunc: persistKeyFunc,
			HandlerFunc:       handlePersist,
		},
		{
			Command:    "expiretime",
			Module:     constants.GenericModule,
			Categories: []string{constants.KeyspaceCategory, constants.ReadCategory, constants.FastCategory},
			Description: `(EXPIRETIME key) Returns the absolute unix time in seconds when the key will expire.
Return -1 if the key exists but has no associated expiry time.
Returns -2 if the key does not exist.`,
			Sync:              false,
			KeyExtractionFunc: expireTimeKeyFunc,
			HandlerFunc:       handleExpireTime,
		},
		{
			Command:    "pexpiretime",
			Module:     constants.GenericModule,
			Categories: []string{constants.KeyspaceCategory, constants.ReadCategory, constants.FastCategory},
			Description: `(PEXPIRETIME key) Returns the absolute unix time in milliseconds when the key will expire.
Return -1 if the key exists but has no associated expiry time.
Returns -2 if the key does not exist.`,
			Sync:              false,
			KeyExtractionFunc: expireTimeKeyFunc,
			HandlerFunc:       handleExpireTime,
		},
		{
			Command:    "ttl",
			Module:     constants.GenericModule,
			Categories: []string{constants.KeyspaceCategory, constants.ReadCategory, constants.FastCategory},
			Description: `(TTL key) Returns the remaining time to live for a key that has an expiry time in seconds.
If the key exists but does not have an associated expiry time, -1 is returned.
If the key does not exist, -2 is returned.`,
			Sync:              false,
			KeyExtractionFunc: ttlKeyFunc,
			HandlerFunc:       handleTTL,
		},
		{
			Command:    "pttl",
			Module:     constants.GenericModule,
			Categories: []string{constants.KeyspaceCategory, constants.ReadCategory, constants.FastCategory},
			Description: `(PTTL key) Returns the remaining time to live for a key that has an expiry time in milliseconds.
If the key exists but does not have an associated expiry time, -1 is returned.
If the key does not exist, -2 is returned.`,
			Sync:              false,
			KeyExtractionFunc: ttlKeyFunc,
			HandlerFunc:       handleTTL,
		},
		{
			Command:    "expire",
			Module:     constants.GenericModule,
			Categories: []string{constants.KeyspaceCategory, constants.WriteCategory, constants.FastCategory},
			Description: `(EXPIRE key seconds [NX | XX | GT | LT])
Expire the key in the specified number of seconds. This commands turns a key into a volatile one.
NX - Only set the expiry time if the key has no associated expiry.
XX - Only set the expiry time if the key already has an expiry time.
GT - Only set the expiry time if the new expiry time is greater than the current one.
LT - Only set the expiry time if the new expiry time is less than the current one.`,
			Sync:              true,
			KeyExtractionFunc: expireKeyFunc,
			HandlerFunc:       handleExpire,
		},
		{
			Command:    "pexpire",
			Module:     constants.GenericModule,
			Categories: []string{constants.KeyspaceCategory, constants.WriteCategory, constants.FastCategory},
			Description: `(PEXPIRE key milliseconds [NX | XX | GT | LT])
Expire the key in the specified number of milliseconds. This commands turns a key into a volatile one.
NX - Only set the expiry time if the key has no associated expiry.
XX - Only set the expiry time if the key already has an expiry time.
GT - Only set the expiry time if the new expiry time is greater than the current one.
LT - Only set the expiry time if the new expiry time is less than the current one.`,
			Sync:              true,
			KeyExtractionFunc: expireKeyFunc,
			HandlerFunc:       handleExpire,
		},
		{
			Command:    "expireat",
			Module:     constants.GenericModule,
			Categories: []string{constants.KeyspaceCategory, constants.WriteCategory, constants.FastCategory},
			Description: `(EXPIREAT key unix-time-seconds [NX | XX | GT | LT])
Expire the key in at the exact unix time in seconds.
This commands turns a key into a volatile one.
NX - Only set the expiry time if the key has no associated expiry.
XX - Only set the expiry time if the key already has an expiry time.
GT - Only set the expiry time if the new expiry time is greater than the current one.
LT - Only set the expiry time if the new expiry time is less than the current one.`,
			Sync:              true,
			KeyExtractionFunc: expireAtKeyFunc,
			HandlerFunc:       handleExpireAt,
		},
		{
			Command:    "pexpireat",
			Module:     constants.GenericModule,
			Categories: []string{constants.KeyspaceCategory, constants.WriteCategory, constants.FastCategory},
			Description: `(PEXPIREAT key unix-time-milliseconds [NX | XX | GT | LT])
Expire the key in at the exact unix time in milliseconds.
This commands turns a key into a volatile one.
NX - Only set the expiry time if the key has no associated expiry.
XX - Only set the expiry time if the key already has an expiry time.
GT - Only set the expiry time if the new expiry time is greater than the current one.
LT - Only set the expiry time if the new expiry time is less than the current one.`,
			Sync:              true,
			KeyExtractionFunc: expireAtKeyFunc,
			HandlerFunc:       handleExpireAt,
		},
		{
			Command:    "incr",
			Module:     constants.GenericModule,
			Categories: []string{constants.KeyspaceCategory, constants.WriteCategory, constants.FastCategory},
			Description: `(INCR key)
Increments the number stored at key by one. If the key does not exist, it is set to 0 before performing the operation.
An error is returned if the key contains a value of the wrong type or contains a string that cannot be represented as integer.
This operation is limited to 64 bit signed integers.`,
			Sync:              true,
			KeyExtractionFunc: incrKeyFunc,
			HandlerFunc:       handleIncr,
		},
		{
			Command:    "decr",
			Module:     constants.GenericModule,
			Categories: []string{constants.KeyspaceCategory, constants.WriteCategory, constants.FastCategory},
			Description: `(DECR key)
Decrements the number stored at key by one.
If the key does not exist, it is set to 0 before performing the operation.
An error is returned if the key contains a value of the wrong type or contains a string that cannot be represented as integer.
This operation is limited to 64 bit signed integers.`,
			Sync:              true,
			KeyExtractionFunc: decrKeyFunc,
			HandlerFunc:       handleDecr,
		},
		{
			Command:    "incrby",
			Module:     constants.GenericModule,
			Categories: []string{constants.KeyspaceCategory, constants.WriteCategory, constants.FastCategory},
			Description: `(INCRBY key increment)
Increments the number stored at key by increment. If the key does not exist, it is set to 0 before performing the operation.
An error is returned if the key contains a value of the wrong type or contains a string that can not be represented as integer.`,
			Sync:              true,
			KeyExtractionFunc: incrByKeyFunc,
			HandlerFunc:       handleIncrBy,
		},
		{
			Command:    "decrby",
			Module:     constants.GenericModule,
			Categories: []string{constants.KeyspaceCategory, constants.WriteCategory, constants.FastCategory},
			Description: `(DECRBY key decrement)
The DECRBY command reduces the value stored at the specified key by the specified decrement.
If the key does not exist, it is initialized with a value of 0 before performing the operation.
If the key's value is not of the correct type or cannot be represented as an integer, an error is returned.`,
			Sync:              true,
			KeyExtractionFunc: decrByKeyFunc,
			HandlerFunc:       handleDecrBy,
		},
		{
			Command:    "rename",
			Module:     constants.GenericModule,
			Categories: []string{constants.KeyspaceCategory, constants.WriteCategory, constants.FastCategory},
			Description: `(RENAME key newkey) 
Renames key to newkey. If newkey already exists, it is overwritten. If key does not exist, an error is returned.`,
			Sync:              true,
			KeyExtractionFunc: renameKeyFunc,
			HandlerFunc:       handleRename,
		},
		{
			Command: "flushall",
			Module:  constants.GenericModule,
			Categories: []string{
				constants.KeyspaceCategory,
				constants.WriteCategory,
				constants.SlowCategory,
				constants.DangerousCategory,
			},
			Description: `(FLUSHALL) Delete all the keys in all the existing databases. This command is always synchronous.`,
			Sync:        true,
			KeyExtractionFunc: func(cmd []string) (internal.KeyExtractionFuncResult, error) {
				return internal.KeyExtractionFuncResult{
					Channels: make([]string, 0), ReadKeys: make([]string, 0), WriteKeys: make([]string, 0),
				}, nil
			},
			HandlerFunc: handleFlush,
		},
		{
			Command: "flushdb",
			Module:  constants.GenericModule,
			Categories: []string{
				constants.KeyspaceCategory,
				constants.WriteCategory,
				constants.SlowCategory,
				constants.DangerousCategory,
			},
			Description: `(FLUSHDB) 
Delete all the keys in the currently selected database. This command is always synchronous.`,
			Sync: true,
			KeyExtractionFunc: func(cmd []string) (internal.KeyExtractionFuncResult, error) {
				return internal.KeyExtractionFuncResult{
					Channels: make([]string, 0), ReadKeys: make([]string, 0), WriteKeys: make([]string, 0),
				}, nil
			},
			HandlerFunc: handleFlush,
		},
	}
}
