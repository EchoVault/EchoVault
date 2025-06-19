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

package hash_test

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/echovault/sugardb/internal"
	"github.com/echovault/sugardb/internal/clock"
	"github.com/echovault/sugardb/internal/constants"
	"github.com/echovault/sugardb/internal/modules/hash"
	"github.com/echovault/sugardb/pkg/config"
	"github.com/echovault/sugardb/sugardb"
	"github.com/tidwall/resp"
)

func Test_Hash(t *testing.T) {
	mockClock := clock.NewClock()
	port, err := internal.GetFreePort()
	if err != nil {
		t.Error(err)
		return
	}

	mockServer, err := sugardb.NewSugarDB(
		sugardb.WithConfig(config.Config{
			BindAddr:       "localhost",
			Port:           uint16(port),
			DataDir:        "",
			EvictionPolicy: constants.NoEviction,
		}),
	)
	if err != nil {
		t.Error(err)
		return
	}

	go func() {
		mockServer.Start()
	}()

	t.Cleanup(func() {
		mockServer.ShutDown()
	})

	t.Run("Test_HandleHSET", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		// Tests for both HSet and HSetNX
		tests := []struct {
			name             string
			key              string
			presetValue      interface{}
			command          []string
			expectedResponse int // Change count
			expectedValue    map[string]string
			expectedError    error
		}{
			{
				name:             "1. HSETNX set field on non-existent hash map",
				key:              "HsetKey1",
				presetValue:      nil,
				command:          []string{"HSETNX", "HsetKey1", "field1", "value1"},
				expectedResponse: 1,
				expectedValue:    map[string]string{"field1": "value1"},
				expectedError:    nil,
			},
			{
				name:             "2. HSETNX set field on existing hash map",
				key:              "HsetKey2",
				presetValue:      map[string]string{"field1": "value1"},
				command:          []string{"HSETNX", "HsetKey2", "field2", "value2"},
				expectedResponse: 1,
				expectedValue:    map[string]string{"field1": "value1", "field2": "value2"},
				expectedError:    nil,
			},
			{
				name:             "3. HSETNX skips operation when setting on existing field",
				key:              "HsetKey3",
				presetValue:      map[string]string{"field1": "value1"},
				command:          []string{"HSETNX", "HsetKey3", "field1", "value1-new"},
				expectedResponse: 0,
				expectedValue:    map[string]string{"field1": "value1"},
				expectedError:    nil,
			},
			{
				name:             "4. Regular HSET command on non-existent hash map",
				key:              "HsetKey4",
				presetValue:      nil,
				command:          []string{"HSET", "HsetKey4", "field1", "value1", "field2", "value2"},
				expectedResponse: 2,
				expectedValue:    map[string]string{"field1": "value1", "field2": "value2"},
				expectedError:    nil,
			},
			{
				name:             "5. Regular HSET update on existing hash map",
				key:              "HsetKey5",
				presetValue:      map[string]string{"field1": "value1", "field2": "value2"},
				command:          []string{"HSET", "HsetKey5", "field1", "value1-new", "field2", "value2-ne2", "field3", "value3"},
				expectedResponse: 3,
				expectedValue:    map[string]string{"field1": "value1-new", "field2": "value2-ne2", "field3": "value3"},
				expectedError:    nil,
			},
			{
				name:             "6. HSET overwrites when the target key is not a map",
				key:              "HsetKey6",
				presetValue:      "Default preset value",
				command:          []string{"HSET", "HsetKey6", "field1", "value1"},
				expectedResponse: 1,
				expectedValue:    map[string]string{"field1": "value1"},
				expectedError:    nil,
			},
			{
				name:             "7. HSET returns error when there's a mismatch in key/values",
				key:              "HsetKey7",
				presetValue:      nil,
				command:          []string{"HSET", "HsetKey7", "field1", "value1", "field2"},
				expectedResponse: 0,
				expectedValue:    map[string]string{},
				expectedError:    errors.New("each field must have a corresponding value"),
			},
			{
				name:             "8. Command too short",
				key:              "HsetKey8",
				presetValue:      nil,
				command:          []string{"HSET", "field1"},
				expectedResponse: 0,
				expectedValue:    map[string]string{},
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValue != nil {
					var command []resp.Value
					var expected string

					switch test.presetValue.(type) {
					case string:
						command = []resp.Value{
							resp.StringValue("SET"),
							resp.StringValue(test.key),
							resp.StringValue(test.presetValue.(string)),
						}
						expected = "ok"
					case map[string]string:
						command = []resp.Value{resp.StringValue("HSET"), resp.StringValue(test.key)}
						for key, value := range test.presetValue.(map[string]string) {
							command = append(command, []resp.Value{
								resp.StringValue(key),
								resp.StringValue(value)}...,
							)
						}
						expected = strconv.Itoa(len(test.presetValue.(map[string]string)))
					}

					if err = client.WriteArray(command); err != nil {
						t.Error(err)
					}
					res, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if !strings.EqualFold(res.String(), expected) {
						t.Errorf("expected preset response to be \"%s\", got %s", expected, res.String())
					}
				}

				command := make([]resp.Value, len(test.command))
				for i, c := range test.command {
					command[i] = resp.StringValue(c)
				}

				if err = client.WriteArray(command); err != nil {
					t.Error(err)
				}
				res, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if test.expectedError != nil {
					if !strings.Contains(res.Error().Error(), test.expectedError.Error()) {
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), err.Error())
					}
					return
				}

				if res.Integer() != test.expectedResponse {
					t.Errorf("expected response \"%d\", got \"%d\"", test.expectedResponse, res.Integer())
				}

				// Check that all the values are what is expected
				if err := client.WriteArray([]resp.Value{
					resp.StringValue("HGETALL"),
					resp.StringValue(test.key),
				}); err != nil {
					t.Error(err)
				}
				res, _, err = client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				for idx, field := range res.Array() {
					if idx%2 == 0 {
						if res.Array()[idx+1].String() != test.expectedValue[field.String()] {
							t.Errorf(
								"expected value \"%+v\" for field \"%s\", got \"%+v\"",
								test.expectedValue[field.String()], field.String(), res.Array()[idx+1].String(),
							)
						}
					}
				}
			})
		}
	})

	t.Run("Test_HandleHINCRBY", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		// Tests for both HIncrBy and HIncrByFloat
		tests := []struct {
			name             string
			key              string
			presetValue      interface{}
			command          []string
			expectedResponse string // Change count
			expectedValue    map[string]string
			expectedError    error
		}{
			{
				name:             "1. Increment by integer on non-existent hash should create a new one",
				key:              "HincrbyKey1",
				presetValue:      nil,
				command:          []string{"HINCRBY", "HincrbyKey1", "field1", "1"},
				expectedResponse: "1",
				expectedValue:    map[string]string{"field1": "1"},
				expectedError:    nil,
			},
			{
				name:             "2. Increment by float on non-existent hash should create one",
				key:              "HincrbyKey2",
				presetValue:      nil,
				command:          []string{"HINCRBYFLOAT", "HincrbyKey2", "field1", "3.142"},
				expectedResponse: "3.142",
				expectedValue:    map[string]string{"field1": "3.142"},
				expectedError:    nil,
			},
			{
				name:             "3. Increment by integer on existing hash",
				key:              "HincrbyKey3",
				presetValue:      map[string]string{"field1": "1"},
				command:          []string{"HINCRBY", "HincrbyKey3", "field1", "10"},
				expectedResponse: "11",
				expectedValue:    map[string]string{"field1": "11"},
				expectedError:    nil,
			},
			{
				name:             "4. Increment by float on an existing hash",
				key:              "HincrbyKey4",
				presetValue:      map[string]string{"field1": "3.142"},
				command:          []string{"HINCRBYFLOAT", "HincrbyKey4", "field1", "3.142"},
				expectedResponse: "6.284",
				expectedValue:    map[string]string{"field1": "6.284"},
				expectedError:    nil,
			},
			{
				name:             "5. Command too short",
				key:              "HincrbyKey5",
				presetValue:      nil,
				command:          []string{"HINCRBY", "HincrbyKey5"},
				expectedResponse: "0",
				expectedValue:    nil,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "6. Command too long",
				key:              "HincrbyKey6",
				presetValue:      nil,
				command:          []string{"HINCRBY", "HincrbyKey6", "field1", "23", "45"},
				expectedResponse: "0",
				expectedValue:    nil,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "7. Error when increment by float does not pass valid float",
				key:              "HincrbyKey7",
				presetValue:      nil,
				command:          []string{"HINCRBYFLOAT", "HincrbyKey7", "field1", "three point one four two"},
				expectedResponse: "0",
				expectedValue:    nil,
				expectedError:    errors.New("increment must be a float"),
			},
			{
				name:             "8. Error when increment does not pass valid integer",
				key:              "HincrbyKey8",
				presetValue:      nil,
				command:          []string{"HINCRBY", "HincrbyKey8", "field1", "three"},
				expectedResponse: "0",
				expectedValue:    nil,
				expectedError:    errors.New("increment must be an integer"),
			},
			{
				name:             "9. Error when trying to increment on a key that is not a hash",
				key:              "HincrbyKey9",
				presetValue:      "Default value",
				command:          []string{"HINCRBY", "HincrbyKey9", "field1", "3"},
				expectedResponse: "0",
				expectedValue:    nil,
				expectedError:    errors.New("value at HincrbyKey9 is not a hash"),
			},
			{
				name:             "10. Error when trying to increment a hash field that is not a number",
				key:              "HincrbyKey10",
				presetValue:      map[string]string{"field1": "value1"},
				command:          []string{"HINCRBY", "HincrbyKey10", "field1", "3"},
				expectedResponse: "0",
				expectedValue:    nil,
				expectedError:    errors.New("value at field field1 is not a number"),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValue != nil {
					var command []resp.Value
					var expected string

					switch test.presetValue.(type) {
					case string:
						command = []resp.Value{
							resp.StringValue("SET"),
							resp.StringValue(test.key),
							resp.StringValue(test.presetValue.(string)),
						}
						expected = "ok"
					case map[string]string:
						command = []resp.Value{resp.StringValue("HSET"), resp.StringValue(test.key)}
						for key, value := range test.presetValue.(map[string]string) {
							command = append(command, []resp.Value{
								resp.StringValue(key),
								resp.StringValue(value)}...,
							)
						}
						expected = strconv.Itoa(len(test.presetValue.(map[string]string)))
					}

					if err = client.WriteArray(command); err != nil {
						t.Error(err)
					}
					res, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if !strings.EqualFold(res.String(), expected) {
						t.Errorf("expected preset response to be \"%s\", got %s", expected, res.String())
					}
				}

				command := make([]resp.Value, len(test.command))
				for i, c := range test.command {
					command[i] = resp.StringValue(c)
				}

				if err = client.WriteArray(command); err != nil {
					t.Error(err)
				}
				res, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if test.expectedError != nil {
					if !strings.Contains(res.Error().Error(), test.expectedError.Error()) {
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), err.Error())
					}
					return
				}

				if res.String() != test.expectedResponse {
					t.Errorf("expected response \"%s\", got \"%s\"", test.expectedResponse, res.String())
				}

				// Check that all the values are what is expected
				if err := client.WriteArray([]resp.Value{
					resp.StringValue("HGETALL"),
					resp.StringValue(test.key),
				}); err != nil {
					t.Error(err)
				}
				res, _, err = client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				for idx, field := range res.Array() {
					if idx%2 == 0 {
						if res.Array()[idx+1].String() != test.expectedValue[field.String()] {
							t.Errorf(
								"expected value \"%+v\" for field \"%s\", got \"%+v\"",
								test.expectedValue[field.String()], field.String(), res.Array()[idx+1].String(),
							)
						}
					}
				}
			})
		}
	})

	t.Run("Test_HandleHGET", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			key              string
			presetValue      interface{}
			command          []string
			expectedResponse []string // Change count
			expectedValue    map[string]string
			expectedError    error
		}{
			{
				name:             "1. Get values from existing hash.",
				key:              "HgetKey1",
				presetValue:      map[string]string{"field1": "value1", "field2": "365", "field3": "3.142"},
				command:          []string{"HGET", "HgetKey1", "field1", "field2", "field3", "field4"},
				expectedResponse: []string{"value1", "365", "3.142", ""},
				expectedValue:    map[string]string{"field1": "value1", "field2": "365", "field3": "3.142"},
				expectedError:    nil,
			},
			{
				name:             "2. Return nil when attempting to get from non-existed key",
				key:              "HgetKey2",
				presetValue:      nil,
				command:          []string{"HGET", "HgetKey2", "field1"},
				expectedResponse: nil,
				expectedValue:    nil,
				expectedError:    nil,
			},
			{
				name:             "3. Error when trying to get from a value that is not a hash map",
				key:              "HgetKey3",
				presetValue:      "Default Value",
				command:          []string{"HGET", "HgetKey3", "field1"},
				expectedResponse: nil,
				expectedValue:    nil,
				expectedError:    errors.New("value at HgetKey3 is not a hash"),
			},
			{
				name:             "4. Command too short",
				key:              "HgetKey4",
				presetValue:      nil,
				command:          []string{"HGET", "HgetKey4"},
				expectedResponse: nil,
				expectedValue:    nil,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValue != nil {
					var command []resp.Value
					var expected string

					switch test.presetValue.(type) {
					case string:
						command = []resp.Value{
							resp.StringValue("SET"),
							resp.StringValue(test.key),
							resp.StringValue(test.presetValue.(string)),
						}
						expected = "ok"
					case map[string]string:
						command = []resp.Value{resp.StringValue("HSET"), resp.StringValue(test.key)}
						for key, value := range test.presetValue.(map[string]string) {
							command = append(command, []resp.Value{
								resp.StringValue(key),
								resp.StringValue(value)}...,
							)
						}
						expected = strconv.Itoa(len(test.presetValue.(map[string]string)))
					}

					if err = client.WriteArray(command); err != nil {
						t.Error(err)
					}
					res, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if !strings.EqualFold(res.String(), expected) {
						t.Errorf("expected preset response to be \"%s\", got %s", expected, res.String())
					}
				}

				command := make([]resp.Value, len(test.command))
				for i, c := range test.command {
					command[i] = resp.StringValue(c)
				}

				if err = client.WriteArray(command); err != nil {
					t.Error(err)
				}
				res, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if test.expectedError != nil {
					if !strings.Contains(res.Error().Error(), test.expectedError.Error()) {
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), err.Error())
					}
					return
				}

				if test.expectedResponse == nil {
					if !res.IsNull() {
						t.Errorf("expected nil response, got %+v", res)
					}
					return
				}

				for _, item := range res.Array() {
					if !slices.Contains(test.expectedResponse, item.String()) {
						t.Errorf("unexpected element \"%s\" in response", item.String())
					}
				}

				// Check that all the values are what is expected
				if err := client.WriteArray([]resp.Value{
					resp.StringValue("HGETALL"),
					resp.StringValue(test.key),
				}); err != nil {
					t.Error(err)
				}
				res, _, err = client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				for idx, field := range res.Array() {
					if idx%2 == 0 {
						if res.Array()[idx+1].String() != test.expectedValue[field.String()] {
							t.Errorf(
								"expected value \"%+v\" for field \"%s\", got \"%+v\"",
								test.expectedValue[field.String()], field.String(), res.Array()[idx+1].String(),
							)
						}
					}
				}
			})
		}
	})

	t.Run("Test_HandleHMGET", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			key              string
			presetValue      interface{}
			command          []string
			expectedResponse []string // Change count
			expectedValue    map[string]string
			expectedError    error
		}{
			{
				name:             "1. Get values from existing hash.",
				key:              "HmgetKey1",
				presetValue:      map[string]string{"field1": "value1", "field2": "365", "field3": "3.142"},
				command:          []string{"HMGET", "HmgetKey1", "field1", "field2", "field3", "field4"},
				expectedResponse: []string{"value1", "365", "3.142", ""},
				expectedValue:    map[string]string{"field1": "value1", "field2": "365", "field3": "3.142"},
				expectedError:    nil,
			},
			{
				name:             "2. Return nil when attempting to get from non-existed key",
				key:              "HmgetKey2",
				presetValue:      nil,
				command:          []string{"HMGET", "HmgetKey2", "field1"},
				expectedResponse: nil,
				expectedValue:    nil,
				expectedError:    nil,
			},
			{
				name:             "3. Error when trying to get from a value that is not a hash map",
				key:              "HmgetKey3",
				presetValue:      "Default Value",
				command:          []string{"HMGET", "HmgetKey3", "field1"},
				expectedResponse: nil,
				expectedValue:    nil,
				expectedError:    errors.New("value at HmgetKey3 is not a hash"),
			},
			{
				name:             "4. Command too short",
				key:              "HmgetKey4",
				presetValue:      nil,
				command:          []string{"HMGET", "HmgetKey4"},
				expectedResponse: nil,
				expectedValue:    nil,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValue != nil {
					var command []resp.Value
					var expected string

					switch test.presetValue.(type) {
					case string:
						command = []resp.Value{
							resp.StringValue("SET"),
							resp.StringValue(test.key),
							resp.StringValue(test.presetValue.(string)),
						}
						expected = "ok"
					case map[string]string:
						command = []resp.Value{resp.StringValue("HSET"), resp.StringValue(test.key)}
						for key, value := range test.presetValue.(map[string]string) {
							command = append(command, []resp.Value{
								resp.StringValue(key),
								resp.StringValue(value)}...,
							)
						}
						expected = strconv.Itoa(len(test.presetValue.(map[string]string)))
					}

					if err = client.WriteArray(command); err != nil {
						t.Error(err)
					}
					res, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if !strings.EqualFold(res.String(), expected) {
						t.Errorf("expected preset response to be \"%s\", got %s", expected, res.String())
					}
				}

				command := make([]resp.Value, len(test.command))
				for i, c := range test.command {
					command[i] = resp.StringValue(c)
				}

				if err = client.WriteArray(command); err != nil {
					t.Error(err)
				}
				res, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if test.expectedError != nil {
					if !strings.Contains(res.Error().Error(), test.expectedError.Error()) {
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), err.Error())
					}
					return
				}

				if test.expectedResponse == nil {
					if !res.IsNull() {
						t.Errorf("expected nil response, got %+v", res)
					}
					return
				}

				for _, item := range res.Array() {
					if !slices.Contains(test.expectedResponse, item.String()) {
						t.Errorf("unexpected element \"%s\" in response", item.String())
					}
				}

				// Check that all the values are what is expected
				if err := client.WriteArray([]resp.Value{
					resp.StringValue("HGETALL"),
					resp.StringValue(test.key),
				}); err != nil {
					t.Error(err)
				}

				res, _, err = client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				for idx, field := range res.Array() {
					if idx%2 == 0 {
						if res.Array()[idx+1].String() != test.expectedValue[field.String()] {
							t.Errorf(
								"expected value \"%+v\" for field \"%s\", got \"%+v\"",
								test.expectedValue[field.String()], field.String(), res.Array()[idx+1].String(),
							)
						}
					}
				}
			})
		}
	})

	t.Run("Test_HandleHSTRLEN", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			key              string
			presetValue      interface{}
			command          []string
			expectedResponse []int // Change count
			expectedValue    map[string]string
			expectedError    error
		}{
			{
				// Return lengths of field values.
				// If the key does not exist, its length should be 0.
				name:             "1. Return lengths of field values.",
				key:              "HstrlenKey1",
				presetValue:      map[string]string{"field1": "value1", "field2": "123456789", "field3": "3.142"},
				command:          []string{"HSTRLEN", "HstrlenKey1", "field1", "field2", "field3", "field4"},
				expectedResponse: []int{len("value1"), len("123456789"), len("3.142"), 0},
				expectedValue:    map[string]string{"field1": "value1", "field2": "123456789", "field3": "3.142"},
				expectedError:    nil,
			},
			{
				name:             "2. Nil response when trying to get HSTRLEN non-existent key",
				key:              "HstrlenKey2",
				presetValue:      nil,
				command:          []string{"HSTRLEN", "HstrlenKey2", "field1"},
				expectedResponse: nil,
				expectedValue:    nil,
				expectedError:    nil,
			},
			{
				name:             "3. Command too short",
				key:              "HstrlenKey3",
				presetValue:      nil,
				command:          []string{"HSTRLEN", "HstrlenKey3"},
				expectedResponse: nil,
				expectedValue:    nil,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "4. Trying to get lengths on a non hash map returns error",
				key:              "HstrlenKey4",
				presetValue:      "Default value",
				command:          []string{"HSTRLEN", "HstrlenKey4", "field1"},
				expectedResponse: nil,
				expectedValue:    nil,
				expectedError:    errors.New("value at HstrlenKey4 is not a hash"),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValue != nil {
					var command []resp.Value
					var expected string

					switch test.presetValue.(type) {
					case string:
						command = []resp.Value{
							resp.StringValue("SET"),
							resp.StringValue(test.key),
							resp.StringValue(test.presetValue.(string)),
						}
						expected = "ok"
					case map[string]string:
						command = []resp.Value{resp.StringValue("HSET"), resp.StringValue(test.key)}
						for key, value := range test.presetValue.(map[string]string) {
							command = append(command, []resp.Value{
								resp.StringValue(key),
								resp.StringValue(value)}...,
							)
						}
						expected = strconv.Itoa(len(test.presetValue.(map[string]string)))
					}

					if err = client.WriteArray(command); err != nil {
						t.Error(err)
					}
					res, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if !strings.EqualFold(res.String(), expected) {
						t.Errorf("expected preset response to be \"%s\", got %s", expected, res.String())
					}
				}

				command := make([]resp.Value, len(test.command))
				for i, c := range test.command {
					command[i] = resp.StringValue(c)
				}

				if err = client.WriteArray(command); err != nil {
					t.Error(err)
				}
				res, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if test.expectedError != nil {
					if !strings.Contains(res.Error().Error(), test.expectedError.Error()) {
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), err.Error())
					}
					return
				}

				if test.expectedResponse == nil {
					if !res.IsNull() {
						t.Errorf("expected nil response, got %+v", res)
					}
					return
				}

				for _, item := range res.Array() {
					if !slices.Contains(test.expectedResponse, item.Integer()) {
						t.Errorf("unexpected element \"%d\" in response", item.Integer())
					}
				}

				// Check that all the values are what is expected
				if err := client.WriteArray([]resp.Value{
					resp.StringValue("HGETALL"),
					resp.StringValue(test.key),
				}); err != nil {
					t.Error(err)
				}
				res, _, err = client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				for idx, field := range res.Array() {
					if idx%2 == 0 {
						if res.Array()[idx+1].String() != test.expectedValue[field.String()] {
							t.Errorf(
								"expected value \"%+v\" for field \"%s\", got \"%+v\"",
								test.expectedValue[field.String()], field.String(), res.Array()[idx+1].String(),
							)
						}
					}
				}
			})
		}
	})

	t.Run("Test_HandleHVALS", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			key              string
			presetValue      interface{}
			command          []string
			expectedResponse []string
			expectedValue    map[string]string
			expectedError    error
		}{
			{
				name:             "1. Return all the values from a hash",
				key:              "HvalsKey1",
				presetValue:      map[string]string{"field1": "value1", "field2": "123456789", "field3": "3.142"},
				command:          []string{"HVALS", "HvalsKey1"},
				expectedResponse: []string{"value1", "123456789", "3.142"},
				expectedValue:    map[string]string{"field1": "value1", "field2": "123456789", "field3": "3.142"},
				expectedError:    nil,
			},
			{
				name:             "2. Empty array response when trying to get HSTRLEN non-existent key",
				key:              "HvalsKey2",
				presetValue:      nil,
				command:          []string{"HVALS", "HvalsKey2"},
				expectedResponse: []string{},
				expectedValue:    nil,
				expectedError:    nil,
			},
			{
				name:             "3. Command too short",
				key:              "HvalsKey3",
				presetValue:      nil,
				command:          []string{"HVALS"},
				expectedResponse: nil,
				expectedValue:    nil,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "4. Command too long",
				key:              "HvalsKey4",
				presetValue:      nil,
				command:          []string{"HVALS", "HvalsKey4", "HvalsKey4"},
				expectedResponse: nil,
				expectedValue:    nil,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "5. Trying to get lengths on a non hash map returns error",
				key:              "HvalsKey5",
				presetValue:      "Default value",
				command:          []string{"HVALS", "HvalsKey5"},
				expectedResponse: nil,
				expectedValue:    nil,
				expectedError:    errors.New("value at HvalsKey5 is not a hash"),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValue != nil {
					var command []resp.Value
					var expected string

					switch test.presetValue.(type) {
					case string:
						command = []resp.Value{
							resp.StringValue("SET"),
							resp.StringValue(test.key),
							resp.StringValue(test.presetValue.(string)),
						}
						expected = "ok"
					case map[string]string:
						command = []resp.Value{resp.StringValue("HSET"), resp.StringValue(test.key)}
						for key, value := range test.presetValue.(map[string]string) {
							command = append(command, []resp.Value{
								resp.StringValue(key),
								resp.StringValue(value)}...,
							)
						}
						expected = strconv.Itoa(len(test.presetValue.(map[string]string)))
					}

					if err = client.WriteArray(command); err != nil {
						t.Error(err)
					}
					res, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if !strings.EqualFold(res.String(), expected) {
						t.Errorf("expected preset response to be \"%s\", got %s", expected, res.String())
					}
				}

				command := make([]resp.Value, len(test.command))
				for i, c := range test.command {
					command[i] = resp.StringValue(c)
				}

				if err = client.WriteArray(command); err != nil {
					t.Error(err)
				}
				res, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if test.expectedError != nil {
					if !strings.Contains(res.Error().Error(), test.expectedError.Error()) {
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), err.Error())
					}
					return
				}

				if test.expectedResponse == nil {
					if !res.IsNull() {
						t.Errorf("expected nil response, got %+v", res)
					}
					return
				}

				for _, item := range res.Array() {
					if !slices.Contains(test.expectedResponse, item.String()) {
						t.Errorf("unexpected element \"%s\" in response", item.String())
					}
				}
			})
		}
	})

	t.Run("Test_HandleHRANDFIELD", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			key              string
			presetValue      interface{}
			command          []string
			expectedResponse []string
			expectedError    error
		}{
			{
				name:             "1. Get a random field",
				key:              "HrandfieldKey1",
				presetValue:      map[string]string{"field1": "value1", "field2": "123456789", "field3": "3.142"},
				command:          []string{"HRANDFIELD", "HrandfieldKey1"},
				expectedResponse: []string{"field1", "field2", "field3"},
				expectedError:    nil,
			},
			{
				name:             "2. Get a random field with a value",
				key:              "HrandfieldKey2",
				presetValue:      map[string]string{"field1": "value1", "field2": "123456789", "field3": "3.142"},
				command:          []string{"HRANDFIELD", "HrandfieldKey2", "1", "WITHVALUES"},
				expectedResponse: []string{"field1", "value1", "field2", "123456789", "field3", "3.142"},
				expectedError:    nil,
			},
			{
				name: "3.  Get several random fields",
				key:  "HrandfieldKey3",
				presetValue: map[string]string{
					"field1": "value1",
					"field2": "123456789",
					"field3": "3.142",
					"field4": "value4",
					"field5": "value5",
				},
				command:          []string{"HRANDFIELD", "HrandfieldKey3", "3"},
				expectedResponse: []string{"field1", "field2", "field3", "field4", "field5"},
				expectedError:    nil,
			},
			{
				name: "4. Get several random fields with their corresponding values",
				key:  "HrandfieldKey4",
				presetValue: map[string]string{
					"field1": "value1",
					"field2": "123456789",
					"field3": "3.142",
					"field4": "value4",
					"field5": "value5",
				},
				command: []string{"HRANDFIELD", "HrandfieldKey4", "3", "WITHVALUES"},
				expectedResponse: []string{
					"field1", "value1", "field2", "123456789", "field3",
					"3.142", "field4", "value4", "field5", "value5",
				},
				expectedError: nil,
			},
			{
				name: "5. Get the entire hash",
				key:  "HrandfieldKey5",
				presetValue: map[string]string{
					"field1": "value1",
					"field2": "123456789",
					"field3": "3.142",
					"field4": "value4",
					"field5": "value5",
				},
				command:          []string{"HRANDFIELD", "HrandfieldKey5", "5"},
				expectedResponse: []string{"field1", "field2", "field3", "field4", "field5"},
				expectedError:    nil,
			},
			{
				name: "6. Get the entire hash with values",
				key:  "HrandfieldKey5",
				presetValue: map[string]string{
					"field1": "value1",
					"field2": "123456789",
					"field3": "3.142",
					"field4": "value4",
					"field5": "value5",
				},
				command: []string{"HRANDFIELD", "HrandfieldKey5", "5", "WITHVALUES"},
				expectedResponse: []string{
					"field1", "value1", "field2", "123456789", "field3",
					"3.142", "field4", "value4", "field5", "value5",
				},
				expectedError: nil,
			},
			{
				name:          "7. Command too short",
				key:           "HrandfieldKey10",
				presetValue:   nil,
				command:       []string{"HRANDFIELD"},
				expectedError: errors.New(constants.WrongArgsResponse),
			},
			{
				name:          "8. Command too long",
				key:           "HrandfieldKey11",
				presetValue:   nil,
				command:       []string{"HRANDFIELD", "HrandfieldKey11", "HrandfieldKey11", "HrandfieldKey11", "HrandfieldKey11"},
				expectedError: errors.New(constants.WrongArgsResponse),
			},
			{
				name:          "9. Trying to get random field on a non hash map returns error",
				key:           "HrandfieldKey12",
				presetValue:   "Default value",
				command:       []string{"HRANDFIELD", "HrandfieldKey12"},
				expectedError: errors.New("value at HrandfieldKey12 is not a hash"),
			},
			{
				name:          "10. Throw error when count provided is not an integer",
				key:           "HrandfieldKey12",
				presetValue:   "Default value",
				command:       []string{"HRANDFIELD", "HrandfieldKey12", "COUNT"},
				expectedError: errors.New("count must be an integer"),
			},
			{
				name:          "11. If fourth argument is provided, it must be \"WITHVALUES\"",
				key:           "HrandfieldKey12",
				presetValue:   "Default value",
				command:       []string{"HRANDFIELD", "HrandfieldKey12", "10", "FLAG"},
				expectedError: errors.New("result modifier must be withvalues"),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValue != nil {
					var command []resp.Value
					var expected string

					switch test.presetValue.(type) {
					case string:
						command = []resp.Value{
							resp.StringValue("SET"),
							resp.StringValue(test.key),
							resp.StringValue(test.presetValue.(string)),
						}
						expected = "ok"
					case map[string]string:
						command = []resp.Value{resp.StringValue("HSET"), resp.StringValue(test.key)}
						for key, value := range test.presetValue.(map[string]string) {
							command = append(command, []resp.Value{
								resp.StringValue(key),
								resp.StringValue(value)}...,
							)
						}
						expected = strconv.Itoa(len(test.presetValue.(map[string]string)))
					}

					if err = client.WriteArray(command); err != nil {
						t.Error(err)
					}
					res, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if !strings.EqualFold(res.String(), expected) {
						t.Errorf("expected preset response to be \"%s\", got %s", expected, res.String())
					}
				}

				command := make([]resp.Value, len(test.command))
				for i, c := range test.command {
					command[i] = resp.StringValue(c)
				}

				if err = client.WriteArray(command); err != nil {
					t.Error(err)
				}
				res, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if test.expectedError != nil {
					if !strings.Contains(res.Error().Error(), test.expectedError.Error()) {
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), err.Error())
					}
					return
				}

				if test.expectedResponse == nil {
					if !res.IsNull() {
						t.Errorf("expected nil response, got %+v", res)
					}
					return
				}

				for _, item := range res.Array() {
					if !slices.Contains(test.expectedResponse, item.String()) {
						t.Errorf("unexpected element \"%s\" in response", item.String())
					}
				}
			})
		}
	})

	t.Run("Test_HandleHLEN", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			key              string
			presetValue      interface{}
			command          []string
			expectedResponse int // Change count
			expectedError    error
		}{
			{
				name:             "1. Return the correct length of the hash",
				key:              "HlenKey1",
				presetValue:      map[string]string{"field1": "value1", "field2": "123456789", "field3": "3.142"},
				command:          []string{"HLEN", "HlenKey1"},
				expectedResponse: 3,
				expectedError:    nil,
			},
			{
				name:             "2. 0 response when trying to call HLEN on non-existent key",
				key:              "HlenKey2",
				presetValue:      nil,
				command:          []string{"HLEN", "HlenKey2"},
				expectedResponse: 0,
				expectedError:    nil,
			},
			{
				name:             "3. Command too short",
				key:              "HlenKey3",
				presetValue:      nil,
				command:          []string{"HLEN"},
				expectedResponse: 0,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "4. Command too long",
				presetValue:      nil,
				command:          []string{"HLEN", "HlenKey4", "HlenKey4"},
				expectedResponse: 0,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "5. Trying to get lengths on a non hash map returns error",
				key:              "HlenKey5",
				presetValue:      "Default value",
				command:          []string{"HLEN", "HlenKey5"},
				expectedResponse: 0,
				expectedError:    errors.New("value at HlenKey5 is not a hash"),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValue != nil {
					var command []resp.Value
					var expected string

					switch test.presetValue.(type) {
					case string:
						command = []resp.Value{
							resp.StringValue("SET"),
							resp.StringValue(test.key),
							resp.StringValue(test.presetValue.(string)),
						}
						expected = "ok"
					case map[string]string:
						command = []resp.Value{resp.StringValue("HSET"), resp.StringValue(test.key)}
						for key, value := range test.presetValue.(map[string]string) {
							command = append(command, []resp.Value{
								resp.StringValue(key),
								resp.StringValue(value)}...,
							)
						}
						expected = strconv.Itoa(len(test.presetValue.(map[string]string)))
					}

					if err = client.WriteArray(command); err != nil {
						t.Error(err)
					}
					res, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if !strings.EqualFold(res.String(), expected) {
						t.Errorf("expected preset response to be \"%s\", got %s", expected, res.String())
					}
				}

				command := make([]resp.Value, len(test.command))
				for i, c := range test.command {
					command[i] = resp.StringValue(c)
				}

				if err = client.WriteArray(command); err != nil {
					t.Error(err)
				}
				res, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if test.expectedError != nil {
					if !strings.Contains(res.Error().Error(), test.expectedError.Error()) {
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), err.Error())
					}
					return
				}

				if res.Integer() != test.expectedResponse {
					t.Errorf("expected response %d, got %d", test.expectedResponse, res.Integer())
				}
			})
		}
	})

	t.Run("Test_HandleHKeys", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			key              string
			presetValue      interface{}
			command          []string
			expectedResponse []string
			expectedError    error
		}{
			{
				name:             "1. Return an array containing all the keys of the hash",
				key:              "HkeysKey1",
				presetValue:      map[string]string{"field1": "value1", "field2": "123456789", "field3": "3.142"},
				command:          []string{"HKEYS", "HkeysKey1"},
				expectedResponse: []string{"field1", "field2", "field3"},
				expectedError:    nil,
			},
			{
				name:             "2. Empty array response when trying to call HKEYS on non-existent key",
				key:              "HkeysKey2",
				presetValue:      nil,
				command:          []string{"HKEYS", "HkeysKey2"},
				expectedResponse: []string{},
				expectedError:    nil,
			},
			{
				name:             "3. Command too short",
				key:              "HkeysKey3",
				presetValue:      nil,
				command:          []string{"HKEYS"},
				expectedResponse: nil,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "4. Command too long",
				key:              "HkeysKey4",
				presetValue:      nil,
				command:          []string{"HKEYS", "HkeysKey4", "HkeysKey4"},
				expectedResponse: nil,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:          "5. Trying to get lengths on a non hash map returns error",
				key:           "HkeysKey5",
				presetValue:   "Default value",
				command:       []string{"HKEYS", "HkeysKey5"},
				expectedError: errors.New("value at HkeysKey5 is not a hash"),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValue != nil {
					var command []resp.Value
					var expected string

					switch test.presetValue.(type) {
					case string:
						command = []resp.Value{
							resp.StringValue("SET"),
							resp.StringValue(test.key),
							resp.StringValue(test.presetValue.(string)),
						}
						expected = "ok"
					case map[string]string:
						command = []resp.Value{resp.StringValue("HSET"), resp.StringValue(test.key)}
						for key, value := range test.presetValue.(map[string]string) {
							command = append(command, []resp.Value{
								resp.StringValue(key),
								resp.StringValue(value)}...,
							)
						}
						expected = strconv.Itoa(len(test.presetValue.(map[string]string)))
					}

					if err = client.WriteArray(command); err != nil {
						t.Error(err)
					}
					res, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if !strings.EqualFold(res.String(), expected) {
						t.Errorf("expected preset response to be \"%s\", got %s", expected, res.String())
					}
				}

				command := make([]resp.Value, len(test.command))
				for i, c := range test.command {
					command[i] = resp.StringValue(c)
				}

				if err = client.WriteArray(command); err != nil {
					t.Error(err)
				}
				res, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if test.expectedError != nil {
					if !strings.Contains(res.Error().Error(), test.expectedError.Error()) {
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), err.Error())
					}
					return
				}

				for _, item := range res.Array() {
					if !slices.Contains(test.expectedResponse, item.String()) {
						t.Errorf("unexpected value \"%s\" in response", item.String())
					}
				}
			})
		}
	})

	t.Run("Test_HandleHGETALL", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			key              string
			presetValue      interface{}
			command          []string
			expectedResponse hash.Hash
			expectedError    error
		}{
			{
				name:             "1. Return an array containing all the fields and values of the hash",
				key:              "HGetAllKey1",
				presetValue:      hash.Hash{"field1": hash.HashValue{Value: "value1"}, "field2": hash.HashValue{Value: "123456789"}, "field3": hash.HashValue{Value: "3.142"}},
				command:          []string{"HGETALL", "HGetAllKey1"},
				expectedResponse: hash.Hash{"field1": hash.HashValue{Value: "value1"}, "field2": hash.HashValue{Value: "123456789"}, "field3": hash.HashValue{Value: "3.142"}},
				expectedError:    nil,
			},
			{
				name:             "2. Empty array response when trying to call HGETALL on non-existent key",
				key:              "HGetAllKey2",
				presetValue:      nil,
				command:          []string{"HGETALL", "HGetAllKey2"},
				expectedResponse: nil,
				expectedError:    nil,
			},
			{
				name:             "3. Command too short",
				key:              "HGetAllKey3",
				presetValue:      nil,
				command:          []string{"HGETALL"},
				expectedResponse: nil,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "4. Command too long",
				key:              "HGetAllKey4",
				presetValue:      nil,
				command:          []string{"HGETALL", "HGetAllKey4", "HGetAllKey4"},
				expectedResponse: nil,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "5. Trying to get lengths on a non hash map returns error",
				key:              "HGetAllKey5",
				presetValue:      "Default value",
				command:          []string{"HGETALL", "HGetAllKey5"},
				expectedResponse: nil,
				expectedError:    errors.New("value at HGetAllKey5 is not a hash"),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValue != nil {
					var command []resp.Value
					var expected string

					switch test.presetValue.(type) {
					case string:
						command = []resp.Value{
							resp.StringValue("SET"),
							resp.StringValue(test.key),
							resp.StringValue(test.presetValue.(string)),
						}
						expected = "ok"
					case hash.Hash:
						command = []resp.Value{resp.StringValue("HSET"), resp.StringValue(test.key)}
						for key, value := range test.presetValue.(hash.Hash) {
							command = append(command, []resp.Value{
								resp.StringValue(key),
								resp.StringValue(value.Value.(string))}...,
							)
						}
						expected = strconv.Itoa(len(test.presetValue.(hash.Hash)))
					}

					if err = client.WriteArray(command); err != nil {
						t.Error(err)
					}
					res, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if !strings.EqualFold(res.String(), expected) {
						t.Errorf("expected preset response to be \"%s\", got %s", expected, res.String())
					}
				}

				command := make([]resp.Value, len(test.command))
				for i, c := range test.command {
					command[i] = resp.StringValue(c)
				}

				if err = client.WriteArray(command); err != nil {
					t.Error(err)
				}
				res, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if test.expectedError != nil {
					if !strings.Contains(res.Error().Error(), test.expectedError.Error()) {
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), err.Error())
					}
					return
				}

				if test.expectedResponse == nil {
					if len(res.Array()) != 0 {
						t.Errorf("expected response to be empty array, got %+v", res)
					}
					return
				}

				for i, item := range res.Array() {
					if i%2 == 0 {
						field := item.String()
						value := hash.HashValue{Value: res.Array()[i+1].String()}

						if test.expectedResponse[field] != value {
							t.Errorf("expected value at field \"%s\" to be \"%s\", got \"%s\"", field, test.expectedResponse[field], value)
						}
					}
				}

			})
		}
	})

	t.Run("Test_HandleHEXISTS", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			key              string
			presetValue      interface{}
			command          []string
			expectedResponse bool
			expectedError    error
		}{
			{
				name:             "1. Return 1 if the field exists in the hash",
				key:              "HexistsKey1",
				presetValue:      map[string]string{"field1": "value1", "field2": "123456789", "field3": "3.142"},
				command:          []string{"HEXISTS", "HexistsKey1", "field1"},
				expectedResponse: true,
				expectedError:    nil,
			},
			{
				name:             "2. 0 response when trying to call HEXISTS on non-existent key",
				key:              "HexistsKey2",
				presetValue:      nil,
				command:          []string{"HEXISTS", "HexistsKey2", "field1"},
				expectedResponse: false,
				expectedError:    nil,
			},
			{
				name:             "3. Command too short",
				key:              "HexistsKey3",
				presetValue:      nil,
				command:          []string{"HEXISTS", "HexistsKey3"},
				expectedResponse: false,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "4. Command too long",
				key:              "HexistsKey4",
				presetValue:      nil,
				command:          []string{"HEXISTS", "HexistsKey4", "field1", "field2"},
				expectedResponse: false,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "5. Trying to get lengths on a non hash map returns error",
				key:              "HexistsKey5",
				presetValue:      "Default value",
				command:          []string{"HEXISTS", "HexistsKey5", "field1"},
				expectedResponse: false,
				expectedError:    errors.New("value at HexistsKey5 is not a hash"),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValue != nil {
					var command []resp.Value
					var expected string

					switch test.presetValue.(type) {
					case string:
						command = []resp.Value{
							resp.StringValue("SET"),
							resp.StringValue(test.key),
							resp.StringValue(test.presetValue.(string)),
						}
						expected = "ok"
					case map[string]string:
						command = []resp.Value{resp.StringValue("HSET"), resp.StringValue(test.key)}
						for key, value := range test.presetValue.(map[string]string) {
							command = append(command, []resp.Value{
								resp.StringValue(key),
								resp.StringValue(value)}...,
							)
						}
						expected = strconv.Itoa(len(test.presetValue.(map[string]string)))
					}

					if err = client.WriteArray(command); err != nil {
						t.Error(err)
					}
					res, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if !strings.EqualFold(res.String(), expected) {
						t.Errorf("expected preset response to be \"%s\", got %s", expected, res.String())
					}
				}

				command := make([]resp.Value, len(test.command))
				for i, c := range test.command {
					command[i] = resp.StringValue(c)
				}

				if err = client.WriteArray(command); err != nil {
					t.Error(err)
				}
				res, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if test.expectedError != nil {
					if !strings.Contains(res.Error().Error(), test.expectedError.Error()) {
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), err.Error())
					}
					return
				}

				if res.Bool() != test.expectedResponse {
					t.Errorf("expected response to be %v, got %v", test.expectedResponse, res.Bool())
				}
			})
		}
	})

	t.Run("Test_HandleHDEL", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			key              string
			presetValue      interface{}
			command          []string
			expectedResponse int
			expectedValue    map[string]string
			expectedError    error
		}{
			{
				name:             "1. Return count of deleted fields in the specified hash",
				key:              "HdelKey1",
				presetValue:      map[string]string{"field1": "value1", "field2": "123456789", "field3": "3.142", "field7": "value7"},
				command:          []string{"HDEL", "HdelKey1", "field1", "field2", "field3", "field4", "field5", "field6"},
				expectedResponse: 3,
				expectedValue:    map[string]string{"field7": "value7"},
				expectedError:    nil,
			},
			{
				name:             "2. 0 response when passing delete fields that are non-existent on valid hash",
				key:              "HdelKey2",
				presetValue:      map[string]string{"field1": "value1", "field2": "value2", "field3": "value3"},
				command:          []string{"HDEL", "HdelKey2", "field4", "field5", "field6"},
				expectedResponse: 0,
				expectedValue:    map[string]string{"field1": "value1", "field2": "value2", "field3": "value3"},
				expectedError:    nil,
			},
			{
				name:             "3. 0 response when trying to call HDEL on non-existent key",
				key:              "HdelKey3",
				presetValue:      nil,
				command:          []string{"HDEL", "HdelKey3", "field1"},
				expectedResponse: 0,
				expectedValue:    nil,
				expectedError:    nil,
			},
			{
				name:             "4. Command too short",
				key:              "HdelKey4",
				presetValue:      nil,
				command:          []string{"HDEL", "HdelKey4"},
				expectedResponse: 0,
				expectedValue:    nil,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "5. Trying to get lengths on a non hash map returns error",
				key:              "HdelKey5",
				presetValue:      "Default value",
				command:          []string{"HDEL", "HdelKey5", "field1"},
				expectedResponse: 0,
				expectedValue:    nil,
				expectedError:    errors.New("value at HdelKey5 is not a hash"),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValue != nil {
					var command []resp.Value
					var expected string

					switch test.presetValue.(type) {
					case string:
						command = []resp.Value{
							resp.StringValue("SET"),
							resp.StringValue(test.key),
							resp.StringValue(test.presetValue.(string)),
						}
						expected = "ok"
					case map[string]string:
						command = []resp.Value{resp.StringValue("HSET"), resp.StringValue(test.key)}
						for key, value := range test.presetValue.(map[string]string) {
							command = append(command, []resp.Value{
								resp.StringValue(key),
								resp.StringValue(value)}...,
							)
						}
						expected = strconv.Itoa(len(test.presetValue.(map[string]string)))
					}

					if err = client.WriteArray(command); err != nil {
						t.Error(err)
					}
					res, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if !strings.EqualFold(res.String(), expected) {
						t.Errorf("expected preset response to be \"%s\", got %s", expected, res.String())
					}
				}

				command := make([]resp.Value, len(test.command))
				for i, c := range test.command {
					command[i] = resp.StringValue(c)
				}

				if err = client.WriteArray(command); err != nil {
					t.Error(err)
				}
				res, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if test.expectedError != nil {
					if !strings.Contains(res.Error().Error(), test.expectedError.Error()) {
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), err.Error())
					}
					return
				}

				if res.Integer() != test.expectedResponse {
					t.Errorf("expected response %d, got %d", test.expectedResponse, res.Integer())
				}

				for idx, field := range res.Array() {
					if idx%2 == 0 {
						if res.Array()[idx+1].String() != test.expectedValue[field.String()] {
							t.Errorf(
								"expected value \"%+v\" for field \"%s\", got \"%+v\"",
								test.expectedValue[field.String()], field.String(), res.Array()[idx+1].String(),
							)
						}
					}
				}
			})
		}
	})

	t.Run("Test_HandleHEXPIRE", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name          string
			key           string
			presetValue   hash.Hash
			command       []string
			expectedValue string
			expectedError error
		}{

			{
				name: "1. Set expiration for all keys in hash, no options.",
				key:  "HexpireKey1",
				presetValue: hash.Hash{
					"HexpireK1Field1": hash.HashValue{
						Value: "default1",
					},
					"HexpireK1Field2": hash.HashValue{
						Value: "default2",
					},
					"HexpireK1Field3": hash.HashValue{
						Value: "default3",
					},
				},
				command:       []string{"HEXPIRE", "HexpireKey1", "5", "FIELDS", "3", "HexpireK1Field1", "HexpireK1Field2", "HexpireK1Field3"},
				expectedValue: "[1 1 1]",
				expectedError: nil,
			},
			{
				name: "2. Set expiration for one key in hash, no options.",
				key:  "HexpireKey2",
				presetValue: hash.Hash{
					"HexpireK2Field1": hash.HashValue{
						Value: "default1",
					},
				},
				command:       []string{"HEXPIRE", "HexpireKey2", "5", "FIELDS", "1", "HexpireK2Field1"},
				expectedValue: "[1]",
				expectedError: nil,
			},
			{
				name: "3. Set expiration, expireTime already populated, no options.",
				key:  "HexpireKey3",
				presetValue: hash.Hash{
					"HexpireK3Field1": hash.HashValue{
						Value:    "default1",
						ExpireAt: mockClock.Now().Add(500 * time.Second),
					},
				},
				command:       []string{"HEXPIRE", "HexpireKey3", "100", "FIELDS", "1", "HexpireK3Field1"},
				expectedValue: "[1]",
				expectedError: nil,
			},
			{
				name: "4. Set expiration, option NX with no expire time currently set.",
				key:  "HexpireKey4",
				presetValue: hash.Hash{
					"HexpireK4Field1": hash.HashValue{
						Value: "default1",
					},
				},
				command:       []string{"HEXPIRE", "HexpireKey4", "5", "NX", "FIELDS", "1", "HexpireK4Field1"},
				expectedValue: "[1]",
				expectedError: nil,
			},
			{
				name: "5. Set expiration, option NX with an expire time already set.",
				key:  "HexpireKey5",
				presetValue: hash.Hash{
					"HexpireK5Field1": hash.HashValue{
						Value:    "default1",
						ExpireAt: mockClock.Now().Add(500 * time.Second),
					},
				},
				command:       []string{"HEXPIRE", "HexpireKey5", "100", "NX", "FIELDS", "1", "HexpireK5Field1"},
				expectedValue: "[0]",
				expectedError: nil,
			},
			{
				name: "6. Set expiration, option XX with no expire time currently set.",
				key:  "HexpireKey6",
				presetValue: hash.Hash{
					"HexpireK6Field1": hash.HashValue{
						Value: "default1",
					},
				},
				command:       []string{"HEXPIRE", "HexpireKey6", "5", "XX", "FIELDS", "1", "HexpireK6Field1"},
				expectedValue: "[0]",
				expectedError: nil,
			},
			{
				name: "7. Set expiration, option XX with expire time already set.",
				key:  "HexpireKey7",
				presetValue: hash.Hash{
					"HexpireK7Field1": hash.HashValue{
						Value:    "default1",
						ExpireAt: mockClock.Now().Add(500 * time.Second),
					},
				},
				command:       []string{"HEXPIRE", "HexpireKey7", "100", "XX", "FIELDS", "1", "HexpireK7Field1"},
				expectedValue: "[1]",
				expectedError: nil,
			},
			{
				name: "8. Set expiration, option GT with expire time less than one provided.",
				key:  "HexpireKey8",
				presetValue: hash.Hash{
					"HexpireK8Field1": hash.HashValue{
						Value:    "default1",
						ExpireAt: mockClock.Now().Add(500 * time.Second),
					},
				},
				command:       []string{"HEXPIRE", "HexpireKey8", "1000", "GT", "FIELDS", "1", "HexpireK8Field1"},
				expectedValue: "[1]",
				expectedError: nil,
			},
			{
				name: "9. Set expiration, option GT with expire time greater than one provided.",
				key:  "HexpireKey9",
				presetValue: hash.Hash{
					"HexpireK9Field1": hash.HashValue{
						Value:    "default1",
						ExpireAt: mockClock.Now().Add(500 * time.Second),
					},
				},
				command:       []string{"HEXPIRE", "HexpireKey9", "100", "GT", "FIELDS", "1", "HexpireK9Field1"},
				expectedValue: "[0]",
				expectedError: nil,
			},
			{
				name: "10. Set expiration, option LT with expire time less than one provided.",
				key:  "HexpireKey10",
				presetValue: hash.Hash{
					"HexpireK10Field1": hash.HashValue{
						Value:    "default1",
						ExpireAt: mockClock.Now().Add(500 * time.Second),
					},
				},
				command:       []string{"HEXPIRE", "HexpireKey10", "1000", "LT", "FIELDS", "1", "HexpireK10Field1"},
				expectedValue: "[0]",
				expectedError: nil,
			},
			{
				name: "11. Set expiration, option LT with expire time greater than one provided.",
				key:  "HexpireKey11",
				presetValue: hash.Hash{
					"HexpireK11Field1": hash.HashValue{
						Value:    "default1",
						ExpireAt: mockClock.Now().Add(500 * time.Second),
					},
				},
				command:       []string{"HEXPIRE", "HexpireKey11", "100", "LT", "FIELDS", "1", "HexpireK11Field1"},
				expectedValue: "[1]",
				expectedError: nil,
			},
			{
				name: "12. Set expiration, provide 0 seconds.",
				key:  "HexpireKey12",
				presetValue: hash.Hash{
					"HexpireK12Field1": hash.HashValue{
						Value: "default1",
					},
				},
				command:       []string{"HEXPIRE", "HexpireKey12", "0", "FIELDS", "1", "HexpireK12Field1"},
				expectedValue: "[2]",
				expectedError: nil,
			},
			{
				name:          "13. Attempt to set expiration for non existent key.",
				key:           "HexpireKeyNOTEXIST",
				presetValue:   nil,
				command:       []string{"HEXPIRE", "HexpireKeyNOTEXIST", "100", "FIELDS", "1", "HexpireKNEField1"},
				expectedValue: "[-2]",
				expectedError: nil,
			},
			{
				name: "14. Attempt to set expiration for field that doesn't exist.",
				key:  "HexpireKey14",
				presetValue: hash.Hash{
					"HexpireK14Field1": hash.HashValue{
						Value: "default1",
					},
				},
				command:       []string{"HEXPIRE", "HexpireKey14", "100", "FIELDS", "2", "HexpireK14BadField1", "HexpireK14Field1"},
				expectedValue: "[-2 1]",
				expectedError: nil,
			},
			{
				name: "15. Set expiration, command wrong length.",
				key:  "HexpireKey15",
				presetValue: hash.Hash{
					"HexpireK15Field1": hash.HashValue{
						Value: "default1",
					},
				},
				command:       []string{"HEXPIRE", "HexpireKey15", "100", "1", "HexpireK15Field1"},
				expectedError: errors.New("Error wrong number of arguments"),
			},
			{
				name: "16. Set expiration, command filed numfields is not a number.",
				key:  "HexpireKey16",
				presetValue: hash.Hash{
					"HexpireK16Field1": hash.HashValue{
						Value: "default1",
					},
				},
				command:       []string{"HEXPIRE", "HexpireKey16", "100", "FIELDS", "one", "HexpireK16Field1"},
				expectedError: errors.New("Error numberfields must be integer, was provided \"one\""),
			},
		}

		for _, test := range tests {

			t.Run(test.name, func(t *testing.T) {
				// set key with preset value
				if test.presetValue != nil {
					var command []resp.Value
					var expected string

					command = []resp.Value{resp.StringValue("HSET"), resp.StringValue(test.key)}
					for key, value := range test.presetValue {
						command = append(command, []resp.Value{
							resp.StringValue(key),
							resp.StringValue(value.Value.(string))}...,
						)
					}
					expected = strconv.Itoa(len(test.presetValue))

					if err = client.WriteArray(command); err != nil {
						t.Error(err)
					}
					res, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if !strings.EqualFold(res.String(), expected) {
						t.Errorf("expected preset response to be \"%s\", got %s", expected, res.String())
					}

				}

				// preset Expire Time
				for field, value := range test.presetValue {
					if value.ExpireAt != (time.Time{}) {
						cmd := []resp.Value{
							resp.StringValue("HEXPIRE"),
							resp.StringValue(test.key),
							resp.StringValue("500"),
							resp.StringValue("FIELDS"),
							resp.StringValue("1"),
							resp.StringValue(field),
						}

						if err = client.WriteArray(cmd); err != nil {
							t.Error(err)
						}
						res, _, err := client.ReadValue()
						if err != nil {
							t.Error(err)
						}
						if res.String() != "[1]" {
							t.Errorf("Error presetting expire time - Key: %s, Field: %s,  response: %s", test.key, field, res.String())
						}
					}
				}

				// run HEXPIRE command
				command := make([]resp.Value, len(test.command))
				for i, c := range test.command {
					command[i] = resp.StringValue(c)
				}

				if err = client.WriteArray(command); err != nil {
					t.Error(err)
				}
				res, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if test.expectedError != nil {
					if !strings.Contains(res.Error().Error(), test.expectedError.Error()) {
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), res.Error())
					}
					return
				}

				if res.String() != test.expectedValue {
					t.Errorf("expected response %q, got %q", test.expectedValue, res.String())
				}

			})

		}

	})
	t.Run("Test_HandleHEXPIREAT", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name          string
			key           string
			presetValue   hash.Hash
			command       []string
			expectedValue string
			expectedError error
		}{

			{
				name: "1. Set expiration for all keys in hash, no options.",
				key:  "HexpireAtKey1",
				presetValue: hash.Hash{
					"HexpireK1Field1": hash.HashValue{
						Value: "default1",
					},
					"HexpireK1Field2": hash.HashValue{
						Value: "default2",
					},
					"HexpireK1Field3": hash.HashValue{
						Value: "default3",
					},
				},
				command:       []string{"HEXPIREAT", "HexpireAtKey1", strconv.FormatInt(mockClock.Now().Unix()+5, 10), "FIELDS", "3", "HexpireK1Field1", "HexpireK1Field2", "HexpireK1Field3"},
				expectedValue: "[1 1 1]",
				expectedError: nil,
			},
			{
				name: "2. Set expiration for one key in hash, no options.",
				key:  "HexpireAtKey2",
				presetValue: hash.Hash{
					"HexpireK2Field1": hash.HashValue{
						Value: "default1",
					},
				},
				command:       []string{"HEXPIREAT", "HexpireAtKey2", strconv.FormatInt(mockClock.Now().Unix()+5, 10), "FIELDS", "1", "HexpireK2Field1"},
				expectedValue: "[1]",
				expectedError: nil,
			},
			{
				name: "3. Set expiration, expireTime already populated, no options.",
				key:  "HexpireAtKey3",
				presetValue: hash.Hash{
					"HexpireK3Field1": hash.HashValue{
						Value:    "default1",
						ExpireAt: mockClock.Now().Add(500 * time.Second),
					},
				},
				command:       []string{"HEXPIREAT", "HexpireAtKey3", strconv.FormatInt(mockClock.Now().Unix()+100, 10), "FIELDS", "1", "HexpireK3Field1"},
				expectedValue: "[1]",
				expectedError: nil,
			},
			{
				name: "4. Set expiration, option NX with no expire time currently set.",
				key:  "HexpireAtKey4",
				presetValue: hash.Hash{
					"HexpireK4Field1": hash.HashValue{
						Value: "default1",
					},
				},
				command:       []string{"HEXPIREAT", "HexpireAtKey4", strconv.FormatInt(mockClock.Now().Unix()+5, 10), "NX", "FIELDS", "1", "HexpireK4Field1"},
				expectedValue: "[1]",
				expectedError: nil,
			},
			{
				name: "5. Set expiration, option NX with an expire time already set.",
				key:  "HexpireAtKey5",
				presetValue: hash.Hash{
					"HexpireK5Field1": hash.HashValue{
						Value:    "default1",
						ExpireAt: mockClock.Now().Add(500 * time.Second),
					},
				},
				command:       []string{"HEXPIREAT", "HexpireAtKey5", strconv.FormatInt(mockClock.Now().Unix()+100, 10), "NX", "FIELDS", "1", "HexpireK5Field1"},
				expectedValue: "[0]",
				expectedError: nil,
			},
			{
				name: "6. Set expiration, option XX with no expire time currently set.",
				key:  "HexpireAtKey6",
				presetValue: hash.Hash{
					"HexpireK6Field1": hash.HashValue{
						Value: "default1",
					},
				},
				command:       []string{"HEXPIREAT", "HexpireAtKey6", strconv.FormatInt(mockClock.Now().Unix()+5, 10), "XX", "FIELDS", "1", "HexpireK6Field1"},
				expectedValue: "[0]",
				expectedError: nil,
			},
			{
				name: "7. Set expiration, option XX with expire time already set.",
				key:  "HexpireAtKey7",
				presetValue: hash.Hash{
					"HexpireK7Field1": hash.HashValue{
						Value:    "default1",
						ExpireAt: mockClock.Now().Add(500 * time.Second),
					},
				},
				command:       []string{"HEXPIREAT", "HexpireAtKey7", strconv.FormatInt(mockClock.Now().Unix()+100, 10), "XX", "FIELDS", "1", "HexpireK7Field1"},
				expectedValue: "[1]",
				expectedError: nil,
			},
			{
				name: "8. Set expiration, option GT with expire time less than one provided.",
				key:  "HexpireAtKey8",
				presetValue: hash.Hash{
					"HexpireK8Field1": hash.HashValue{
						Value:    "default1",
						ExpireAt: mockClock.Now().Add(500 * time.Second),
					},
				},
				command:       []string{"HEXPIREAT", "HexpireAtKey8", strconv.FormatInt(mockClock.Now().Unix()+1000, 10), "GT", "FIELDS", "1", "HexpireK8Field1"},
				expectedValue: "[1]",
				expectedError: nil,
			},
			{
				name: "9. Set expiration, option GT with expire time greater than one provided.",
				key:  "HexpireAtKey9",
				presetValue: hash.Hash{
					"HexpireK9Field1": hash.HashValue{
						Value:    "default1",
						ExpireAt: mockClock.Now().Add(500 * time.Second),
					},
				},
				command:       []string{"HEXPIREAT", "HexpireAtKey9", strconv.FormatInt(mockClock.Now().Unix()+100, 10), "GT", "FIELDS", "1", "HexpireK9Field1"},
				expectedValue: "[0]",
				expectedError: nil,
			},
			{
				name: "10. Set expiration, option LT with expire time less than one provided.",
				key:  "HexpireAtKey10",
				presetValue: hash.Hash{
					"HexpireK10Field1": hash.HashValue{
						Value:    "default1",
						ExpireAt: mockClock.Now().Add(500 * time.Second),
					},
				},
				command:       []string{"HEXPIREAT", "HexpireAtKey10", strconv.FormatInt(mockClock.Now().Unix()+1000, 10), "LT", "FIELDS", "1", "HexpireK10Field1"},
				expectedValue: "[0]",
				expectedError: nil,
			},
			{
				name: "11. Set expiration, option LT with expire time greater than one provided.",
				key:  "HexpireAtKey11",
				presetValue: hash.Hash{
					"HexpireK11Field1": hash.HashValue{
						Value:    "default1",
						ExpireAt: mockClock.Now().Add(500 * time.Second),
					},
				},
				command:       []string{"HEXPIREAT", "HexpireAtKey11", strconv.FormatInt(mockClock.Now().Unix()+100, 10), "LT", "FIELDS", "1", "HexpireK11Field1"},
				expectedValue: "[1]",
				expectedError: nil,
			},
			{
				name: "12. Set expiration, provide 0 seconds.",
				key:  "HexpireAtKey12",
				presetValue: hash.Hash{
					"HexpireK12Field1": hash.HashValue{
						Value: "default1",
					},
				},
				command:       []string{"HEXPIREAT", "HexpireAtKey12", strconv.FormatInt(mockClock.Now().Unix()-10, 10), "FIELDS", "1", "HexpireK12Field1"},
				expectedValue: "[2]",
				expectedError: nil,
			},
			{
				name:          "13. Attempt to set expiration for non existent key.",
				key:           "HexpireAtKeyNOTEXIST",
				presetValue:   nil,
				command:       []string{"HEXPIREAT", "HexpireAtKeyNOTEXIST", strconv.FormatInt(mockClock.Now().Unix()+10, 10), "FIELDS", "1", "HexpireKNEField1"},
				expectedValue: "[-2]",
				expectedError: nil,
			},
			{
				name: "14. Attempt to set expiration for field that doesn't exist.",
				key:  "HexpireAtKey14",
				presetValue: hash.Hash{
					"HexpireK14Field1": hash.HashValue{
						Value: "default1",
					},
				},
				command:       []string{"HEXPIREAT", "HexpireAtKey14", strconv.FormatInt(mockClock.Now().Unix()+10, 10), "FIELDS", "2", "HexpireK14BadField1", "HexpireK14Field1"},
				expectedValue: "[-2 1]",
				expectedError: nil,
			},
			{
				name: "15. Set expiration, command wrong length.",
				key:  "HexpireAtKey15",
				presetValue: hash.Hash{
					"HexpireK15Field1": hash.HashValue{
						Value: "default1",
					},
				},
				command:       []string{"HEXPIREAT", "HexpireAtKey15", strconv.FormatInt(mockClock.Now().Unix()+10, 10), "1", "HexpireK15Field1"},
				expectedError: errors.New("Error wrong number of arguments"),
			},
			{
				name: "16. Set expiration, command filed numfields is not a number.",
				key:  "HexpireAtKey16",
				presetValue: hash.Hash{
					"HexpireK16Field1": hash.HashValue{
						Value: "default1",
					},
				},
				command:       []string{"HEXPIREAT", "HexpireAtKey16", strconv.FormatInt(mockClock.Now().Unix()+10, 10), "FIELDS", "one", "HexpireK16Field1"},
				expectedError: errors.New("Error numberfields must be integer, was provided \"one\""),
			},
		}

		for _, test := range tests {

			t.Run(test.name, func(t *testing.T) {
				// set key with preset value
				if test.presetValue != nil {
					var command []resp.Value
					var expected string

					command = []resp.Value{resp.StringValue("HSET"), resp.StringValue(test.key)}
					for key, value := range test.presetValue {
						command = append(command, []resp.Value{
							resp.StringValue(key),
							resp.StringValue(value.Value.(string))}...,
						)
					}
					expected = strconv.Itoa(len(test.presetValue))

					if err = client.WriteArray(command); err != nil {
						t.Error(err)
					}
					res, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if !strings.EqualFold(res.String(), expected) {
						t.Errorf("expected preset response to be \"%s\", got %s", expected, res.String())
					}

				}

				// preset Expire Time
				for field, value := range test.presetValue {
					if value.ExpireAt != (time.Time{}) {
						cmd := []resp.Value{
							resp.StringValue("HEXPIRE"),
							resp.StringValue(test.key),
							resp.StringValue("500"),
							resp.StringValue("FIELDS"),
							resp.StringValue("1"),
							resp.StringValue(field),
						}

						if err = client.WriteArray(cmd); err != nil {
							t.Error(err)
						}
						res, _, err := client.ReadValue()
						if err != nil {
							t.Error(err)
						}
						if res.String() != "[1]" {
							t.Errorf("Error presetting expire time - Key: %s, Field: %s,  response: %s", test.key, field, res.String())
						}
					}
				}

				// run HEXPIREAT command
				command := make([]resp.Value, len(test.command))
				for i, c := range test.command {
					command[i] = resp.StringValue(c)
				}

				if err = client.WriteArray(command); err != nil {
					t.Error(err)
				}
				res, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if test.expectedError != nil {
					if !strings.Contains(res.Error().Error(), test.expectedError.Error()) {
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), res.Error())
					}
					return
				}

				if res.String() != test.expectedValue {
					t.Errorf("expected response %q, got %q", test.expectedValue, res.String())
				}

			})

		}

	})

	t.Run("Test_HandleHTTL", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name          string
			command       []string
			key           string
			presetValue   interface{}
			setExpire     bool
			expectedValue string
			expectedError error
		}{
			{
				name:    "1. Get TTL for one field when expireTime is set.",
				key:     "HTTLKey1",
				command: []string{"HTTL", "HTTLKey1", "FIELDS", "1", "HTTLK1Field1"},
				presetValue: hash.Hash{
					"HTTLK1Field1": hash.HashValue{
						Value: "default1",
					},
				},
				setExpire:     true,
				expectedValue: "[5]",
				expectedError: nil,
			},
			{
				name:    "2. Get TTL for multiple fields when expireTime is set.",
				key:     "HTTLKey2",
				command: []string{"HTTL", "HTTLKey2", "FIELDS", "3", "HTTLK2Field1", "HTTLK2Field2", "HTTLK2Field3"},
				presetValue: hash.Hash{
					"HTTLK2Field1": hash.HashValue{
						Value: "default1",
					},
					"HTTLK2Field2": hash.HashValue{
						Value: "default1",
					},
					"HTTLK2Field3": hash.HashValue{
						Value: "default1",
					},
				},
				setExpire:     true,
				expectedValue: "[5 5 5]",
				expectedError: nil,
			},
			{
				name:    "3. Get TTL for one field when expireTime is not set.",
				key:     "HTTLKey3",
				command: []string{"HTTL", "HTTLKey3", "FIELDS", "1", "HTTLK3Field1"},
				presetValue: hash.Hash{
					"HTTLK3Field1": hash.HashValue{
						Value: "default1",
					},
				},
				setExpire:     false,
				expectedValue: "[-1]",
				expectedError: nil,
			},
			{
				name:    "4. Get TTL for multiple fields when expireTime is not set.",
				key:     "HTTLKey4",
				command: []string{"HTTL", "HTTLKey4", "FIELDS", "3", "HTTLK4Field1", "HTTLK4Field2", "HTTLK4Field3"},
				presetValue: hash.Hash{
					"HTTLK4Field1": hash.HashValue{
						Value: "default1",
					},
					"HTTLK4Field2": hash.HashValue{
						Value: "default1",
					},
					"HTTLK4Field3": hash.HashValue{
						Value: "default1",
					},
				},
				setExpire:     false,
				expectedValue: "[-1 -1 -1]",
				expectedError: nil,
			},
			{
				name:          "5. Try to get TTL for key that doesn't exist.",
				key:           "HTTLKeyNOTEXIST",
				command:       []string{"HTTL", "HTTLKeyNOTEXIST", "FIELDS", "1", "HTTLK1Field1"},
				presetValue:   nil,
				setExpire:     false,
				expectedValue: "[-2]",
				expectedError: nil,
			},
			{
				name:          "6. Try to get TTL for key that isn't a hash.",
				key:           "HTTLKey6",
				command:       []string{"HTTL", "HTTLKey6", "FIELDS", "1", "HTTLK6Field1"},
				presetValue:   "NotaHash",
				setExpire:     false,
				expectedError: errors.New("Error value at HTTLKey6 is not a hash"),
			},
			{
				name:    "7. Command missing 'FIELDS'.",
				key:     "HTTLKey7",
				command: []string{"HTTL", "HTTLKey7", "1", "HTTLK7Field1"},
				presetValue: hash.Hash{
					"HTTLK7Field1": hash.HashValue{
						Value: "default1",
					},
				},
				setExpire:     false,
				expectedError: errors.New("Error wrong number of arguments"),
			},
			{
				name:    "8. Command numfields provided isn't a number.",
				key:     "HTTLKey8",
				command: []string{"HTTL", "HTTLKey8", "FIELDS", "one", "HTTLK8Field1"},
				presetValue: hash.Hash{
					"HTTLK8Field1": hash.HashValue{
						Value: "default1",
					},
				},
				setExpire:     false,
				expectedError: errors.New("Error expire time must be integer, was provided \"one\""),
			},
			{
				name:    "9. Command missing numfields.",
				key:     "HTTLKey9",
				command: []string{"HTTL", "HTTLKey9", "FIELDS", "HTTLK9Field1"},
				presetValue: hash.Hash{
					"HTTLK9Field1": hash.HashValue{
						Value: "default1",
					},
				},
				setExpire:     false,
				expectedError: errors.New("Error wrong number of arguments"),
			},
			{
				name:    "10. Command FIELDS index contains something else.",
				key:     "HTTLKey10",
				command: []string{"HTTL", "HTTLKey10", "NOTFIELDS", "1", "HTTLK10Field1"},
				presetValue: hash.Hash{
					"HTTLK10Field1": hash.HashValue{
						Value: "default1",
					},
				},
				setExpire:     false,
				expectedError: errors.New("Error invalid command provided"),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				// set preset values
				if test.presetValue != nil {
					var command []resp.Value
					var expected string

					switch test.presetValue.(type) {
					case string:
						command = []resp.Value{
							resp.StringValue("SET"),
							resp.StringValue(test.key),
							resp.StringValue(test.presetValue.(string)),
						}
						expected = "ok"
					case hash.Hash:
						command = []resp.Value{resp.StringValue("HSET"), resp.StringValue(test.key)}
						for key, value := range test.presetValue.(hash.Hash) {
							command = append(command, []resp.Value{
								resp.StringValue(key),
								resp.StringValue(value.Value.(string))}...,
							)
						}
						expected = strconv.Itoa(len(test.presetValue.(hash.Hash)))
					}

					if err = client.WriteArray(command); err != nil {
						t.Error(err)
					}

					res, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if !strings.EqualFold(res.String(), expected) {
						t.Errorf("expected preset response to be \"%s\", got %s", expected, res.String())
					}

				}

				if test.setExpire {
					// set expire times
					command := make([]resp.Value, len(test.presetValue.(hash.Hash))+5)
					command[0] = resp.StringValue("HEXPIRE")
					command[1] = resp.StringValue(test.key)
					command[2] = resp.StringValue("5")
					command[3] = resp.StringValue("FIELDS")
					command[4] = resp.StringValue(fmt.Sprintf("%v", (len(test.presetValue.(hash.Hash)))))

					i := 0
					for k, _ := range test.presetValue.(hash.Hash) {
						command[5+i] = resp.StringValue(k)
						i++
					}

					if err = client.WriteArray(command); err != nil {
						t.Error(err)
					}
					_, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
					}
				}

				// read TTL
				command := make([]resp.Value, len(test.command))
				for i, v := range test.command {
					command[i] = resp.StringValue(v)
				}
				if err = client.WriteArray(command); err != nil {
					t.Error(err)
				}
				resp, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if test.expectedError != nil {
					if !strings.Contains(resp.Error().Error(), test.expectedError.Error()) {
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), resp.Error())
					}

					return
				}

				if resp.String() != test.expectedValue {
					t.Errorf("Expected value %v but got %v", test.expectedValue, resp)
				}

			})

		}

	})

	t.Run("Test_HandleHPEXPIRETIME", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		const fixedTimestamp = 1136189545000

		tests := []struct {
			name          string
			key           string
			command       []string
			presetValue   interface{}
			setExpire     bool
			expireSeconds int64
			expectedValue string
			expectedError error
		}{
			{
				name:    "1. Single field with expiration",
				key:     "HPExpireTimeKey1",
				command: []string{"HPEXPIRETIME", "HPExpireTimeKey1", "FIELDS", "1", "field1"},
				presetValue: hash.Hash{
					"field1": hash.HashValue{
						Value: "default1",
					},
				},
				setExpire:     true,
				expireSeconds: 500,
				expectedValue: fmt.Sprintf("[%d]", fixedTimestamp),
			},
			{
				name:    "2. Single field with no expiration",
				key:     "HPExpireTimeKey2",
				command: []string{"HPEXPIRETIME", "HPExpireTimeKey2", "FIELDS", "1", "field1"},
				presetValue: hash.Hash{
					"field1": hash.HashValue{
						Value: "default1",
					},
				},
				setExpire:     false,
				expectedValue: "[-1]",
			},
			{
				name:    "3. Multiple fields mixed",
				key:     "HPExpireTimeKey3",
				command: []string{"HPEXPIRETIME", "HPExpireTimeKey3", "FIELDS", "3", "field1", "field2", "nonexist"},
				presetValue: hash.Hash{
					"field1": hash.HashValue{
						Value: "default1",
					},
					"field2": hash.HashValue{
						Value: "default2",
					},
				},
				setExpire:     true,
				expireSeconds: 500,
				expectedValue: fmt.Sprintf("[%d %d -2]", fixedTimestamp, fixedTimestamp),
			},
			{
				name:          "4. Key does not exist",
				key:           "NonExistentKey",
				command:       []string{"HPEXPIRETIME", "NonExistentKey", "FIELDS", "1", "field1"},
				presetValue:   nil,
				setExpire:     false,
				expectedValue: "-1",
			},
			{
				name:          "5. Key is not a hash",
				key:           "HPExpireTimeKey5",
				command:       []string{"HPEXPIRETIME", "HPExpireTimeKey5", "FIELDS", "1", "field1"},
				presetValue:   "string value",
				setExpire:     false,
				expectedValue: "",
				expectedError: errors.New("value at HPExpireTimeKey5 is not a hash"),
			},
			{
				name:    "6. Invalid numfields format",
				key:     "HPExpireTimeKey6",
				command: []string{"HPEXPIRETIME", "HPExpireTimeKey6", "FIELDS", "notanumber", "field1"},
				presetValue: hash.Hash{
					"field1": hash.HashValue{
						Value: "default1",
					},
				},
				setExpire:     false,
				expectedValue: "",
				expectedError: errors.New("expire time must be integer, was provided \"notanumber\""),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValue != nil {
					var command []resp.Value

					switch v := test.presetValue.(type) {
					case string:
						command = []resp.Value{
							resp.StringValue("SET"),
							resp.StringValue(test.key),
							resp.StringValue(v),
						}
					case hash.Hash:
						command = []resp.Value{resp.StringValue("HSET"), resp.StringValue(test.key)}
						for key, value := range v {
							command = append(command, resp.StringValue(key), resp.StringValue(value.Value.(string)))
						}
					}

					if err = client.WriteArray(command); err != nil {
						t.Error(err)
					}
					if _, _, err = client.ReadValue(); err != nil {
						t.Error(err)
					}

					if test.setExpire {
						if hash, ok := test.presetValue.(hash.Hash); ok {
							for field := range hash {
								expireCmd := []resp.Value{
									resp.StringValue("HEXPIRE"),
									resp.StringValue(test.key),
									resp.StringValue(strconv.FormatInt(test.expireSeconds, 10)),
									resp.StringValue("FIELDS"),
									resp.StringValue("1"),
									resp.StringValue(field),
								}
								if err = client.WriteArray(expireCmd); err != nil {
									t.Error(err)
								}
								if _, _, err = client.ReadValue(); err != nil {
									t.Error(err)
								}
							}
						}
					}
				}

				// Execute HPEXPIRETIME command
				command := make([]resp.Value, len(test.command))
				for i, v := range test.command {
					command[i] = resp.StringValue(v)
				}
				if err = client.WriteArray(command); err != nil {
					t.Error(err)
				}

				resp, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if test.expectedError != nil {
					if !strings.Contains(resp.Error().Error(), test.expectedError.Error()) {
						t.Errorf("expected error %q, got %q", test.expectedError.Error(), resp.Error())
					}
					return
				}

				if resp.String() != test.expectedValue {
					t.Errorf("Expected value %q but got %q", test.expectedValue, resp.String())
				}
			})
		}
	})

	t.Run("Test_HandleHEXPIRETIME", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		const fixedTimestamp = 1136189545

		tests := []struct {
			name          string
			key           string
			command       []string
			presetValue   interface{}
			setExpire     bool
			expireSeconds int64
			expectedValue string
			expectedError error
		}{
			{
				name:    "1. Single field with expiration",
				key:     "HExpireTimeKey1",
				command: []string{"HEXPIRETIME", "HExpireTimeKey1", "FIELDS", "1", "field1"},
				presetValue: hash.Hash{
					"field1": hash.HashValue{
						Value: "default1",
					},
				},
				setExpire:     true,
				expireSeconds: 500,
				expectedValue: fmt.Sprintf("[%d]", fixedTimestamp),
			},
			{
				name:    "2. Single field with no expiration",
				key:     "HExpireTimeKey2",
				command: []string{"HEXPIRETIME", "HExpireTimeKey2", "FIELDS", "1", "field1"},
				presetValue: hash.Hash{
					"field1": hash.HashValue{
						Value: "default1",
					},
				},
				setExpire:     false,
				expectedValue: "[-1]",
			},
			{
				name:    "3. Multiple fields mixed",
				key:     "HExpireTimeKey3",
				command: []string{"HEXPIRETIME", "HExpireTimeKey3", "FIELDS", "3", "field1", "field2", "nonexist"},
				presetValue: hash.Hash{
					"field1": hash.HashValue{
						Value: "default1",
					},
					"field2": hash.HashValue{
						Value: "default2",
					},
				},
				setExpire:     true,
				expireSeconds: 500,
				expectedValue: fmt.Sprintf("[%d %d -2]", fixedTimestamp, fixedTimestamp),
			},
			{
				name:          "4. Key does not exist",
				key:           "NonExistentKey",
				command:       []string{"HEXPIRETIME", "NonExistentKey", "FIELDS", "1", "field1"},
				presetValue:   nil,
				setExpire:     false,
				expectedValue: "-1",
			},
			{
				name:          "5. Key is not a hash",
				key:           "HExpireTimeKey5",
				command:       []string{"HEXPIRETIME", "HExpireTimeKey5", "FIELDS", "1", "field1"},
				presetValue:   "string value",
				setExpire:     false,
				expectedValue: "",
				expectedError: errors.New("value at HExpireTimeKey5 is not a hash"),
			},
			{
				name:    "6. Invalid numfields format",
				key:     "HExpireTimeKey6",
				command: []string{"HEXPIRETIME", "HExpireTimeKey6", "FIELDS", "notanumber", "field1"},
				presetValue: hash.Hash{
					"field1": hash.HashValue{
						Value: "default1",
					},
				},
				setExpire:     false,
				expectedValue: "",
				expectedError: errors.New("expire time must be integer, was provided \"notanumber\""),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValue != nil {
					var command []resp.Value

					switch v := test.presetValue.(type) {
					case string:
						command = []resp.Value{
							resp.StringValue("SET"),
							resp.StringValue(test.key),
							resp.StringValue(v),
						}
					case hash.Hash:
						command = []resp.Value{resp.StringValue("HSET"), resp.StringValue(test.key)}
						for key, value := range v {
							command = append(command, resp.StringValue(key), resp.StringValue(value.Value.(string)))
						}
					}

					if err = client.WriteArray(command); err != nil {
						t.Error(err)
					}
					if _, _, err = client.ReadValue(); err != nil {
						t.Error(err)
					}

					if test.setExpire {
						if hash, ok := test.presetValue.(hash.Hash); ok {
							for field := range hash {
								expireCmd := []resp.Value{
									resp.StringValue("HEXPIRE"),
									resp.StringValue(test.key),
									resp.StringValue(strconv.FormatInt(test.expireSeconds, 10)),
									resp.StringValue("FIELDS"),
									resp.StringValue("1"),
									resp.StringValue(field),
								}
								if err = client.WriteArray(expireCmd); err != nil {
									t.Error(err)
								}
								if _, _, err = client.ReadValue(); err != nil {
									t.Error(err)
								}
							}
						}
					}
				}

				command := make([]resp.Value, len(test.command))
				for i, v := range test.command {
					command[i] = resp.StringValue(v)
				}
				if err = client.WriteArray(command); err != nil {
					t.Error(err)
				}

				resp, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if test.expectedError != nil {
					if !strings.Contains(resp.Error().Error(), test.expectedError.Error()) {
						t.Errorf("expected error %q, got %q", test.expectedError.Error(), resp.Error())
					}
					return
				}

				if resp.String() != test.expectedValue {
					t.Errorf("Expected value %q but got %q", test.expectedValue, resp.String())
				}
			})
		}
	})
}
