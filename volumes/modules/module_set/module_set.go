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
	"fmt"
	"strconv"
	"strings"
)

var Command string = "Module.Set"

var Categories []string = []string{"write", "fast"}

var Description string = `(Module.Set key value) This module stores the given value at the specified key.
The value must be an integer`

var Sync bool = true

func KeyExtractionFunc(cmd []string, args ...string) ([]string, []string, error) {
	if len(cmd) != 3 {
		return nil, nil, fmt.Errorf("wrong no of args for %s command", strings.ToLower(Command))
	}
	return []string{}, cmd[1:2], nil
}

func HandlerFunc(
	ctx context.Context,
	command []string,
	keysExist func(ctx context.Context, keys []string) map[string]bool,
	getValues func(ctx context.Context, keys []string) map[string]interface{},
	setValues func(ctx context.Context, entries map[string]interface{}) error,
	args ...string) ([]byte, error) {

	_, writeKeys, err := KeyExtractionFunc(command, args...)
	if err != nil {
		return nil, err
	}
	key := writeKeys[0]

	value, err := strconv.ParseInt(command[2], 10, 64)
	if err != nil {
		return nil, err
	}

	err = setValues(ctx, map[string]interface{}{key: value})
	if err != nil {
		return nil, err
	}

	return []byte("+OK\r\n"), nil
}
