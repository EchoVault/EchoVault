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

package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
)

var Command string = "Module.Get"

var Categories []string = []string{"read", "fast"}

var Description string = `(Module.Get key) This module fetches the integer value from the key and returns the value ^ 2.
0 is returned if the key does not exist. An error is returned if the value is not an integer.`

var Sync bool = false

func KeyExtractionFunc(cmd []string, args ...string) ([]string, []string, error) {
	if len(cmd) != 2 {
		return nil, nil, fmt.Errorf("wrong no of args for %s command", Command)
	}
	return cmd[1:], []string{}, nil
}

func HandlerFunc(
	ctx context.Context,
	command []string,
	keysExist func(ctx context.Context, keys []string) map[string]bool,
	getValues func(ctx context.Context, keys []string) map[string]interface{},
	setValues func(ctx context.Context, entries map[string]interface{}) error,
	args ...string) ([]byte, error) {

	readKeys, _, err := KeyExtractionFunc(command, args...)
	if err != nil {
		return nil, err
	}
	key := readKeys[0]
	exists := keysExist(ctx, readKeys)[key]

	if !exists {
		return []byte(":0\r\n"), nil
	}

	val, ok := getValues(ctx, []string{key})[key].(int64)
	if !ok {
		return nil, fmt.Errorf("value at key %s is not an integer", key)
	}

	factor := val
	if len(args) >= 1 {
		factor, err = strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return nil, errors.New("first value of args must be an integer")
		}
	}

	return []byte(fmt.Sprintf(":%d\r\n", val*factor)), nil
}
