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

package set_test

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/echovault/sugardb/internal"
	"github.com/echovault/sugardb/internal/constants"
	"github.com/echovault/sugardb/internal/modules/set"
	"github.com/echovault/sugardb/pkg/config"
	"github.com/echovault/sugardb/sugardb"
	"github.com/tidwall/resp"
)

func Test_Set(t *testing.T) {
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

	t.Run("Test_HandleSADD", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error()
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			preset           bool
			presetValue      interface{}
			key              string
			command          []string
			expectedValue    *set.Set
			expectedResponse int
			expectedError    error
		}{
			{
				name:             "1. Create new set on a non-existent key, return count of added elements",
				preset:           false,
				presetValue:      nil,
				key:              "SaddKey1",
				command:          []string{"SADD", "SaddKey1", "one", "two", "three", "four"},
				expectedValue:    set.NewSet([]string{"one", "two", "three", "four"}),
				expectedResponse: 4,
				expectedError:    nil,
			},
			{
				name:             "2. Add members to an exiting set, skip members that already exist in the set, return added count.",
				preset:           true,
				presetValue:      set.NewSet([]string{"one", "two", "three", "four"}),
				key:              "SaddKey2",
				command:          []string{"SADD", "SaddKey2", "three", "four", "five", "six", "seven"},
				expectedValue:    set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven"}),
				expectedResponse: 3,
				expectedError:    nil,
			},
			{
				name:             "3. Throw error when trying to add to a key that does not hold a set",
				preset:           true,
				presetValue:      "Default value",
				key:              "SaddKey3",
				command:          []string{"SADD", "SaddKey3", "member"},
				expectedResponse: 0,
				expectedError:    errors.New("value at key SaddKey3 is not a set"),
			},
			{
				name:             "4. Command too short",
				preset:           false,
				key:              "SaddKey4",
				command:          []string{"SADD", "SaddKey4"},
				expectedValue:    nil,
				expectedResponse: 0,
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
					case *set.Set:
						command = []resp.Value{resp.StringValue("SADD"), resp.StringValue(test.key)}
						for _, element := range test.presetValue.(*set.Set).GetAll() {
							command = append(command, []resp.Value{resp.StringValue(element)}...)
						}
						expected = strconv.Itoa(test.presetValue.(*set.Set).Cardinality())
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

				// Check if the resulting set(s) contain the expected members.
				if test.expectedValue == nil {
					return
				}

				if err := client.WriteArray([]resp.Value{resp.StringValue("SMEMBERS"), resp.StringValue(test.key)}); err != nil {
					t.Error(err)
				}
				res, _, err = client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if len(res.Array()) != test.expectedValue.Cardinality() {
					t.Errorf("expected set at key \"%s\" to have cardinality %d, got %d",
						test.key, test.expectedValue.Cardinality(), len(res.Array()))
				}

				for _, item := range res.Array() {
					if !test.expectedValue.Contains(item.String()) {
						t.Errorf("unexpected memeber \"%s\", in response", item.String())
					}
				}
			})
		}
	})

	t.Run("Test_HandleSCARD", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error()
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			presetValue      interface{}
			key              string
			command          []string
			expectedValue    *set.Set
			expectedResponse int
			expectedError    error
		}{
			{
				name:             "1. Get cardinality of valid set.",
				presetValue:      set.NewSet([]string{"one", "two", "three", "four"}),
				key:              "ScardKey1",
				command:          []string{"SCARD", "ScardKey1"},
				expectedValue:    nil,
				expectedResponse: 4,
				expectedError:    nil,
			},
			{
				name:             "2. Return 0 when trying to get cardinality on non-existent key",
				presetValue:      nil,
				key:              "ScardKey2",
				command:          []string{"SCARD", "ScardKey2"},
				expectedValue:    nil,
				expectedResponse: 0,
				expectedError:    nil,
			},
			{
				name:             "3. Throw error when trying to get cardinality of a value that is not a set",
				presetValue:      "Default value",
				key:              "ScardKey3",
				command:          []string{"SCARD", "ScardKey3"},
				expectedResponse: 0,
				expectedError:    errors.New("value at key ScardKey3 is not a set"),
			},
			{
				name:             "4. Command too short",
				key:              "ScardKey4",
				command:          []string{"SCARD"},
				expectedValue:    nil,
				expectedResponse: 0,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "5. Command too long",
				key:              "ScardKey5",
				command:          []string{"SCARD", "ScardKey5", "ScardKey5"},
				expectedValue:    nil,
				expectedResponse: 0,
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
					case *set.Set:
						command = []resp.Value{resp.StringValue("SADD"), resp.StringValue(test.key)}
						for _, element := range test.presetValue.(*set.Set).GetAll() {
							command = append(command, []resp.Value{resp.StringValue(element)}...)
						}
						expected = strconv.Itoa(test.presetValue.(*set.Set).Cardinality())
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
			})
		}
	})

	t.Run("Test_HandleSDIFF", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error()
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			presetValues     map[string]interface{}
			command          []string
			expectedResponse []string
			expectedError    error
		}{
			{
				name: "1. Get the difference between 2 sets.",
				presetValues: map[string]interface{}{
					"SdiffKey1": set.NewSet([]string{"one", "two", "three", "four", "five"}),
					"SdiffKey2": set.NewSet([]string{"three", "four", "five", "six", "seven", "eight"}),
				},
				command:          []string{"SDIFF", "SdiffKey1", "SdiffKey2"},
				expectedResponse: []string{"one", "two"},
				expectedError:    nil,
			},
			{
				name: "2. Get the difference between 3 sets.",
				presetValues: map[string]interface{}{
					"SdiffKey3": set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven", "eight"}),
					"SdiffKey4": set.NewSet([]string{"one", "two", "thirty-six", "twelve", "eleven"}),
					"SdiffKey5": set.NewSet([]string{"seven", "eight", "nine", "ten", "twelve"}),
				},
				command:          []string{"SDIFF", "SdiffKey3", "SdiffKey4", "SdiffKey5"},
				expectedResponse: []string{"three", "four", "five", "six"},
				expectedError:    nil,
			},
			{
				name: "3. Return base set element if base set is the only valid set",
				presetValues: map[string]interface{}{
					"SdiffKey6": set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven", "eight"}),
					"SdiffKey7": "Default value",
					"SdiffKey8": "123456789",
				},
				command:          []string{"SDIFF", "SdiffKey6", "SdiffKey7", "SdiffKey8"},
				expectedResponse: []string{"one", "two", "three", "four", "five", "six", "seven", "eight"},
				expectedError:    nil,
			},
			{
				name: "4. Throw error when base set is not a set.",
				presetValues: map[string]interface{}{
					"SdiffKey9":  "Default value",
					"SdiffKey10": set.NewSet([]string{"one", "two", "thirty-six", "twelve", "eleven"}),
					"SdiffKey11": set.NewSet([]string{"seven", "eight", "nine", "ten", "twelve"}),
				},
				command:          []string{"SDIFF", "SdiffKey9", "SdiffKey10", "SdiffKey11"},
				expectedResponse: nil,
				expectedError:    errors.New("value at key SdiffKey9 is not a set"),
			},
			{
				name: "5. Throw error when base set is non-existent.",
				presetValues: map[string]interface{}{
					"SdiffKey12": set.NewSet([]string{"one", "two", "thirty-six", "twelve", "eleven"}),
					"SdiffKey13": set.NewSet([]string{"seven", "eight", "nine", "ten", "twelve"}),
				},
				command:          []string{"SDIFF", "non-existent", "SdiffKey7", "SdiffKey8"},
				expectedResponse: nil,
				expectedError:    errors.New("key for base set \"non-existent\" does not exist"),
			},
			{
				name:             "6. Command too short",
				command:          []string{"SDIFF"},
				expectedResponse: []string{},
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValues != nil {
					var command []resp.Value
					var expected string
					for key, value := range test.presetValues {
						switch value.(type) {
						case string:
							command = []resp.Value{
								resp.StringValue("SET"),
								resp.StringValue(key),
								resp.StringValue(value.(string)),
							}
							expected = "ok"
						case *set.Set:
							command = []resp.Value{resp.StringValue("SADD"), resp.StringValue(key)}
							for _, element := range value.(*set.Set).GetAll() {
								command = append(command, []resp.Value{resp.StringValue(element)}...)
							}
							expected = strconv.Itoa(value.(*set.Set).Cardinality())
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

				if len(res.Array()) != len(test.expectedResponse) {
					t.Errorf("expected response array of length \"%d\", got \"%d\"",
						len(test.expectedResponse), len(res.Array()))
				}

				for _, item := range res.Array() {
					if !slices.Contains(test.expectedResponse, item.String()) {
						t.Errorf("unexpected element \"%s\" in response", item.String())
					}
				}
			})
		}
	})

	t.Run("Test_HandleSDIFFSTORE", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error()
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			presetValues     map[string]interface{}
			destination      string
			command          []string
			expectedValue    *set.Set
			expectedResponse int
			expectedError    error
		}{
			{
				name: "1. Get the difference between 2 sets.",
				presetValues: map[string]interface{}{
					"SdiffStoreKey1": set.NewSet([]string{"one", "two", "three", "four", "five"}),
					"SdiffStoreKey2": set.NewSet([]string{"three", "four", "five", "six", "seven", "eight"}),
				},
				destination:      "SdiffStoreDestination1",
				command:          []string{"SDIFFSTORE", "SdiffStoreDestination1", "SdiffStoreKey1", "SdiffStoreKey2"},
				expectedValue:    set.NewSet([]string{"one", "two"}),
				expectedResponse: 2,
				expectedError:    nil,
			},
			{
				name: "2. Get the difference between 3 sets.",
				presetValues: map[string]interface{}{
					"SdiffStoreKey3": set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven", "eight"}),
					"SdiffStoreKey4": set.NewSet([]string{"one", "two", "thirty-six", "twelve", "eleven"}),
					"SdiffStoreKey5": set.NewSet([]string{"seven", "eight", "nine", "ten", "twelve"}),
				},
				destination:      "SdiffStoreDestination2",
				command:          []string{"SDIFFSTORE", "SdiffStoreDestination2", "SdiffStoreKey3", "SdiffStoreKey4", "SdiffStoreKey5"},
				expectedValue:    set.NewSet([]string{"three", "four", "five", "six"}),
				expectedResponse: 4,
				expectedError:    nil,
			},
			{
				name: "3. Return base set element if base set is the only valid set",
				presetValues: map[string]interface{}{
					"SdiffStoreKey6": set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven", "eight"}),
					"SdiffStoreKey7": "Default value",
					"SdiffStoreKey8": "123456789",
				},
				destination:      "SdiffStoreDestination3",
				command:          []string{"SDIFFSTORE", "SdiffStoreDestination3", "SdiffStoreKey6", "SdiffStoreKey7", "SdiffStoreKey8"},
				expectedValue:    set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven", "eight"}),
				expectedResponse: 8,
				expectedError:    nil,
			},
			{
				name: "4. Throw error when base set is not a set.",
				presetValues: map[string]interface{}{
					"SdiffStoreKey9":  "Default value",
					"SdiffStoreKey10": set.NewSet([]string{"one", "two", "thirty-six", "twelve", "eleven"}),
					"SdiffStoreKey11": set.NewSet([]string{"seven", "eight", "nine", "ten", "twelve"}),
				},
				destination:      "SdiffStoreDestination4",
				command:          []string{"SDIFFSTORE", "SdiffStoreDestination4", "SdiffStoreKey9", "SdiffStoreKey10", "SdiffStoreKey11"},
				expectedValue:    nil,
				expectedResponse: 0,
				expectedError:    errors.New("value at key SdiffStoreKey9 is not a set"),
			},
			{
				name:        "5. Throw error when base set is non-existent.",
				destination: "SdiffStoreDestination5",
				presetValues: map[string]interface{}{
					"SdiffStoreKey12": set.NewSet([]string{"one", "two", "thirty-six", "twelve", "eleven"}),
					"SdiffStoreKey13": set.NewSet([]string{"seven", "eight", "nine", "ten", "twelve"}),
				},
				command:          []string{"SDIFFSTORE", "SdiffStoreDestination5", "non-existent", "SdiffStoreKey7", "SdiffStoreKey8"},
				expectedValue:    nil,
				expectedResponse: 0,
				expectedError:    errors.New("key for base set \"non-existent\" does not exist"),
			},
			{
				name:             "6. Command too short",
				command:          []string{"SDIFFSTORE", "SdiffStoreDestination6"},
				expectedResponse: 0,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValues != nil {
					var command []resp.Value
					var expected string
					for key, value := range test.presetValues {
						switch value.(type) {
						case string:
							command = []resp.Value{
								resp.StringValue("SET"),
								resp.StringValue(key),
								resp.StringValue(value.(string)),
							}
							expected = "ok"
						case *set.Set:
							command = []resp.Value{resp.StringValue("SADD"), resp.StringValue(key)}
							for _, element := range value.(*set.Set).GetAll() {
								command = append(command, []resp.Value{resp.StringValue(element)}...)
							}
							expected = strconv.Itoa(value.(*set.Set).Cardinality())
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

				// Check if the resulting set(s) contain the expected members.
				if test.expectedValue == nil {
					return
				}

				if err := client.WriteArray([]resp.Value{
					resp.StringValue("SMEMBERS"),
					resp.StringValue(test.destination),
				}); err != nil {
					t.Error(err)
				}
				res, _, err = client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if len(res.Array()) != test.expectedValue.Cardinality() {
					t.Errorf("expected set at key \"%s\" to have cardinality %d, got %d",
						test.destination, test.expectedValue.Cardinality(), len(res.Array()))
				}

				for _, item := range res.Array() {
					if !test.expectedValue.Contains(item.String()) {
						t.Errorf("unexpected memeber \"%s\", in response", item.String())
					}
				}
			})
		}
	})

	t.Run("Test_HandleSINTER", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error()
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			presetValues     map[string]interface{}
			command          []string
			expectedResponse []string
			expectedError    error
		}{
			{
				name: "1. Get the intersection between 2 sets.",
				presetValues: map[string]interface{}{
					"SinterKey1": set.NewSet([]string{"one", "two", "three", "four", "five"}),
					"SinterKey2": set.NewSet([]string{"three", "four", "five", "six", "seven", "eight"}),
				},
				command:          []string{"SINTER", "SinterKey1", "SinterKey2"},
				expectedResponse: []string{"three", "four", "five"},
				expectedError:    nil,
			},
			{
				name: "2. Get the intersection between 3 sets.",
				presetValues: map[string]interface{}{
					"SinterKey3": set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven", "eight"}),
					"SinterKey4": set.NewSet([]string{"one", "two", "thirty-six", "twelve", "eleven", "eight"}),
					"SinterKey5": set.NewSet([]string{"one", "eight", "nine", "ten", "twelve"}),
				},
				command:          []string{"SINTER", "SinterKey3", "SinterKey4", "SinterKey5"},
				expectedResponse: []string{"one", "eight"},
				expectedError:    nil,
			},
			{
				name: "3. Throw an error if any of the provided keys are not sets",
				presetValues: map[string]interface{}{
					"SinterKey6": set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven", "eight"}),
					"SinterKey7": "Default value",
					"SinterKey8": set.NewSet([]string{"one"}),
				},
				command:          []string{"SINTER", "SinterKey6", "SinterKey7", "SinterKey8"},
				expectedResponse: nil,
				expectedError:    errors.New("value at key SinterKey7 is not a set"),
			},
			{
				name: "4. Throw error when base set is not a set.",
				presetValues: map[string]interface{}{
					"SinterKey9":  "Default value",
					"SinterKey10": set.NewSet([]string{"one", "two", "thirty-six", "twelve", "eleven"}),
					"SinterKey11": set.NewSet([]string{"seven", "eight", "nine", "ten", "twelve"}),
				},
				command:          []string{"SINTER", "SinterKey9", "SinterKey10", "SinterKey11"},
				expectedResponse: nil,
				expectedError:    errors.New("value at key SinterKey9 is not a set"),
			},
			{
				name: "5. If any of the keys does not exist, return an empty array.",
				presetValues: map[string]interface{}{
					"SinterKey12": set.NewSet([]string{"one", "two", "thirty-six", "twelve", "eleven"}),
					"SinterKey13": set.NewSet([]string{"seven", "eight", "nine", "ten", "twelve"}),
				},
				command:          []string{"SINTER", "non-existent", "SinterKey7", "SinterKey8"},
				expectedResponse: []string{},
				expectedError:    nil,
			},
			{
				name:             "6. Command too short",
				command:          []string{"SINTER"},
				expectedResponse: []string{},
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValues != nil {
					var command []resp.Value
					var expected string
					for key, value := range test.presetValues {
						switch value.(type) {
						case string:
							command = []resp.Value{
								resp.StringValue("SET"),
								resp.StringValue(key),
								resp.StringValue(value.(string)),
							}
							expected = "ok"
						case *set.Set:
							command = []resp.Value{resp.StringValue("SADD"), resp.StringValue(key)}
							for _, element := range value.(*set.Set).GetAll() {
								command = append(command, []resp.Value{resp.StringValue(element)}...)
							}
							expected = strconv.Itoa(value.(*set.Set).Cardinality())
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

				if len(res.Array()) != len(test.expectedResponse) {
					t.Errorf("expected response array of length \"%d\", got \"%d\"",
						len(test.expectedResponse), len(res.Array()))
				}

				for _, item := range res.Array() {
					if !slices.Contains(test.expectedResponse, item.String()) {
						t.Errorf("unexpected element \"%s\" in response", item.String())
					}
				}
			})
		}
	})

	t.Run("Test_HandleSINTERCARD", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error()
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			presetValues     map[string]interface{}
			command          []string
			expectedResponse int
			expectedError    error
		}{
			{
				name: "1. Get the full intersect cardinality between 2 sets.",
				presetValues: map[string]interface{}{
					"SinterCardKey1": set.NewSet([]string{"one", "two", "three", "four", "five"}),
					"SinterCardKey2": set.NewSet([]string{"three", "four", "five", "six", "seven", "eight"}),
				},
				command:          []string{"SINTERCARD", "SinterCardKey1", "SinterCardKey2"},
				expectedResponse: 3,
				expectedError:    nil,
			},
			{
				name: "2. Get an intersect cardinality between 2 sets with a limit",
				presetValues: map[string]interface{}{
					"SinterCardKey3": set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"}),
					"SinterCardKey4": set.NewSet([]string{"three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven", "twelve"}),
				},
				command:          []string{"SINTERCARD", "SinterCardKey3", "SinterCardKey4", "LIMIT", "3"},
				expectedResponse: 3,
				expectedError:    nil,
			},
			{
				name: "3. Get the full intersect cardinality between 3 sets.",
				presetValues: map[string]interface{}{
					"SinterCardKey5": set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven", "eight"}),
					"SinterCardKey6": set.NewSet([]string{"one", "two", "thirty-six", "twelve", "eleven", "eight"}),
					"SinterCardKey7": set.NewSet([]string{"one", "seven", "eight", "nine", "ten", "twelve"}),
				},
				command:          []string{"SINTERCARD", "SinterCardKey5", "SinterCardKey6", "SinterCardKey7"},
				expectedResponse: 2,
				expectedError:    nil,
			},
			{
				name: "4. Get the intersection of 3 sets with a limit",
				presetValues: map[string]interface{}{
					"SinterCardKey8":  set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven", "eight"}),
					"SinterCardKey9":  set.NewSet([]string{"one", "two", "thirty-six", "twelve", "eleven", "eight"}),
					"SinterCardKey10": set.NewSet([]string{"one", "two", "seven", "eight", "nine", "ten", "twelve"}),
				},
				command:          []string{"SINTERCARD", "SinterCardKey8", "SinterCardKey9", "SinterCardKey10", "LIMIT", "2"},
				expectedResponse: 2,
				expectedError:    nil,
			},
			{
				name: "5. Return 0 if any of the keys does not exist",
				presetValues: map[string]interface{}{
					"SinterCardKey11": set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven", "eight"}),
					"SinterCardKey12": "Default value",
					"SinterCardKey13": set.NewSet([]string{"one"}),
				},
				command:          []string{"SINTERCARD", "SinterCardKey11", "SinterCardKey12", "SinterCardKey13", "non-existent"},
				expectedResponse: 0,
				expectedError:    nil,
			},
			{
				name: "6. Throw error when one of the keys is not a valid set.",
				presetValues: map[string]interface{}{
					"SinterCardKey14": "Default value",
					"SinterCardKey15": set.NewSet([]string{"one", "two", "thirty-six", "twelve", "eleven"}),
					"SinterCardKey16": set.NewSet([]string{"seven", "eight", "nine", "ten", "twelve"}),
				},
				command:          []string{"SINTERCARD", "SinterCardKey14", "SinterCardKey15", "SinterCardKey16"},
				expectedResponse: 0,
				expectedError:    errors.New("value at key SinterCardKey14 is not a set"),
			},
			{
				name:             "7. Command too short",
				command:          []string{"SINTERCARD"},
				expectedResponse: 0,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValues != nil {
					var command []resp.Value
					var expected string
					for key, value := range test.presetValues {
						switch value.(type) {
						case string:
							command = []resp.Value{
								resp.StringValue("SET"),
								resp.StringValue(key),
								resp.StringValue(value.(string)),
							}
							expected = "ok"
						case *set.Set:
							command = []resp.Value{resp.StringValue("SADD"), resp.StringValue(key)}
							for _, element := range value.(*set.Set).GetAll() {
								command = append(command, []resp.Value{resp.StringValue(element)}...)
							}
							expected = strconv.Itoa(value.(*set.Set).Cardinality())
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
					t.Errorf("expected response array of length \"%d\", got \"%d\"", test.expectedResponse, res.Integer())
				}
			})
		}

	})

	t.Run("Test_HandleSINTERSTORE", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error()
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			presetValues     map[string]interface{}
			destination      string
			command          []string
			expectedValue    *set.Set
			expectedResponse int
			expectedError    error
		}{
			{
				name: "1. Get the intersection between 2 sets and store it at the destination.",
				presetValues: map[string]interface{}{
					"SinterStoreKey1": set.NewSet([]string{"one", "two", "three", "four", "five"}),
					"SinterStoreKey2": set.NewSet([]string{"three", "four", "five", "six", "seven", "eight"}),
				},
				destination:      "SinterStoreDestination1",
				command:          []string{"SINTERSTORE", "SinterStoreDestination1", "SinterStoreKey1", "SinterStoreKey2"},
				expectedValue:    set.NewSet([]string{"three", "four", "five"}),
				expectedResponse: 3,
				expectedError:    nil,
			},
			{
				name: "2. Get the intersection between 3 sets and store it at the destination key.",
				presetValues: map[string]interface{}{
					"SinterStoreKey3": set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven", "eight"}),
					"SinterStoreKey4": set.NewSet([]string{"one", "two", "thirty-six", "twelve", "eleven", "eight"}),
					"SinterStoreKey5": set.NewSet([]string{"one", "seven", "eight", "nine", "ten", "twelve"}),
				},
				destination:      "SinterStoreDestination2",
				command:          []string{"SINTERSTORE", "SinterStoreDestination2", "SinterStoreKey3", "SinterStoreKey4", "SinterStoreKey5"},
				expectedValue:    set.NewSet([]string{"one", "eight"}),
				expectedResponse: 2,
				expectedError:    nil,
			},
			{
				name: "3. Throw error when any of the keys is not a set",
				presetValues: map[string]interface{}{
					"SinterStoreKey6": set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven", "eight"}),
					"SinterStoreKey7": "Default value",
					"SinterStoreKey8": set.NewSet([]string{"one"}),
				},
				destination:      "SinterStoreDestination3",
				command:          []string{"SINTERSTORE", "SinterStoreDestination3", "SinterStoreKey6", "SinterStoreKey7", "SinterStoreKey8"},
				expectedValue:    nil,
				expectedResponse: 0,
				expectedError:    errors.New("value at key SinterStoreKey7 is not a set"),
			},
			{
				name: "4. Throw error when base set is not a set.",
				presetValues: map[string]interface{}{
					"SinterStoreKey9":  "Default value",
					"SinterStoreKey10": set.NewSet([]string{"one", "two", "thirty-six", "twelve", "eleven"}),
					"SinterStoreKey11": set.NewSet([]string{"seven", "eight", "nine", "ten", "twelve"}),
				},
				destination:      "SinterStoreDestination4",
				command:          []string{"SINTERSTORE", "SinterStoreDestination4", "SinterStoreKey9", "SinterStoreKey10", "SinterStoreKey11"},
				expectedValue:    nil,
				expectedResponse: 0,
				expectedError:    errors.New("value at key SinterStoreKey9 is not a set"),
			},
			{
				name:        "5. Return an empty intersection if one of the keys does not exist.",
				destination: "SinterStoreDestination5",
				presetValues: map[string]interface{}{
					"SinterStoreKey12": set.NewSet([]string{"one", "two", "thirty-six", "twelve", "eleven"}),
					"SinterStoreKey13": set.NewSet([]string{"seven", "eight", "nine", "ten", "twelve"}),
				},
				command:          []string{"SINTERSTORE", "SinterStoreDestination5", "non-existent", "SinterStoreKey7", "SinterStoreKey8"},
				expectedValue:    nil,
				expectedResponse: 0,
				expectedError:    nil,
			},
			{
				name:             "6. Command too short",
				command:          []string{"SINTERSTORE", "SinterStoreDestination6"},
				expectedResponse: 0,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValues != nil {
					var command []resp.Value
					var expected string
					for key, value := range test.presetValues {
						switch value.(type) {
						case string:
							command = []resp.Value{
								resp.StringValue("SET"),
								resp.StringValue(key),
								resp.StringValue(value.(string)),
							}
							expected = "ok"
						case *set.Set:
							command = []resp.Value{resp.StringValue("SADD"), resp.StringValue(key)}
							for _, element := range value.(*set.Set).GetAll() {
								command = append(command, []resp.Value{resp.StringValue(element)}...)
							}
							expected = strconv.Itoa(value.(*set.Set).Cardinality())
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

				// Check if the resulting set(s) contain the expected members.
				if test.expectedValue == nil {
					return
				}

				if err := client.WriteArray([]resp.Value{
					resp.StringValue("SMEMBERS"),
					resp.StringValue(test.destination),
				}); err != nil {
					t.Error(err)
				}
				res, _, err = client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if len(res.Array()) != test.expectedValue.Cardinality() {
					t.Errorf("expected set at key \"%s\" to have cardinality %d, got %d",
						test.destination, test.expectedValue.Cardinality(), len(res.Array()))
				}

				for _, item := range res.Array() {
					if !test.expectedValue.Contains(item.String()) {
						t.Errorf("unexpected memeber \"%s\", in response", item.String())
					}
				}
			})
		}
	})

	t.Run("Test_HandleSISMEMBER", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error()
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			presetValue      interface{}
			key              string
			command          []string
			expectedResponse int
			expectedError    error
		}{
			{
				name:             "1. Return 1 when element is a member of the set",
				presetValue:      set.NewSet([]string{"one", "two", "three", "four"}),
				key:              "SIsMemberKey1",
				command:          []string{"SISMEMBER", "SIsMemberKey1", "three"},
				expectedResponse: 1,
				expectedError:    nil,
			},
			{
				name:             "2. Return 0 when element is not a member of the set",
				presetValue:      set.NewSet([]string{"one", "two", "three", "four"}),
				key:              "SIsMemberKey2",
				command:          []string{"SISMEMBER", "SIsMemberKey2", "five"},
				expectedResponse: 0,
				expectedError:    nil,
			},
			{
				name:             "3. Throw error when trying to assert membership when the key does not hold a valid set",
				presetValue:      "Default value",
				key:              "SIsMemberKey3",
				command:          []string{"SISMEMBER", "SIsMemberKey3", "one"},
				expectedResponse: 0,
				expectedError:    errors.New("value at key SIsMemberKey3 is not a set"),
			},
			{
				name:             "4. Command too short",
				key:              "SIsMemberKey4",
				command:          []string{"SISMEMBER", "SIsMemberKey4"},
				expectedResponse: 0,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "5. Command too long",
				key:              "SIsMemberKey5",
				command:          []string{"SISMEMBER", "SIsMemberKey5", "one", "two", "three"},
				expectedResponse: 0,
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
					case *set.Set:
						command = []resp.Value{resp.StringValue("SADD"), resp.StringValue(test.key)}
						for _, element := range test.presetValue.(*set.Set).GetAll() {
							command = append(command, []resp.Value{resp.StringValue(element)}...)
						}
						expected = strconv.Itoa(test.presetValue.(*set.Set).Cardinality())
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
			})
		}
	})

	t.Run("Test_HandleSMEMBERS", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error()
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
				name:             "1. Return all the members of the set.",
				key:              "SmembersKey1",
				presetValue:      set.NewSet([]string{"one", "two", "three", "four", "five"}),
				command:          []string{"SMEMBERS", "SmembersKey1"},
				expectedResponse: []string{"one", "two", "three", "four", "five"},
				expectedError:    nil,
			},
			{
				name:             "2. If the key does not exist, return an empty array.",
				key:              "SmembersKey2",
				presetValue:      nil,
				command:          []string{"SMEMBERS", "SmembersKey2"},
				expectedResponse: nil,
				expectedError:    nil,
			},
			{
				name:             "3. Throw error when the provided key is not a set.",
				key:              "SmembersKey3",
				presetValue:      "Default value",
				command:          []string{"SMEMBERS", "SmembersKey3"},
				expectedResponse: nil,
				expectedError:    errors.New("value at key SmembersKey3 is not a set"),
			},
			{
				name:             "4. Command too short",
				command:          []string{"SMEMBERS"},
				expectedResponse: []string{},
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "5. Command too long",
				command:          []string{"SMEMBERS", "SmembersKey5", "SmembersKey6"},
				expectedResponse: []string{},
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
					case *set.Set:
						command = []resp.Value{resp.StringValue("SADD"), resp.StringValue(test.key)}
						for _, element := range test.presetValue.(*set.Set).GetAll() {
							command = append(command, []resp.Value{resp.StringValue(element)}...)
						}
						expected = strconv.Itoa(test.presetValue.(*set.Set).Cardinality())
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

				if len(res.Array()) != len(test.expectedResponse) {
					t.Errorf("expected response array of length \"%d\", got \"%d\"",
						len(test.expectedResponse), len(res.Array()))
				}

				for _, item := range res.Array() {
					if !slices.Contains(test.expectedResponse, item.String()) {
						t.Errorf("unexpected element \"%s\" in response", item.String())
					}
				}
			})
		}
	})

	t.Run("Test_HandleSMISMEMBER", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error()
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			presetValue      interface{}
			key              string
			command          []string
			expectedResponse []int
			expectedError    error
		}{
			{
				// 1. Return set membership status for multiple elements
				// Return 1 for present and 0 for absent
				// The placement of the membership status flag should me consistent with the order the elements
				// are in within the original command
				name:             "1. Return set membership status for multiple elements",
				presetValue:      set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven"}),
				key:              "SmismemberKey1",
				command:          []string{"SMISMEMBER", "SmismemberKey1", "three", "four", "five", "six", "eight", "nine", "seven"},
				expectedResponse: []int{1, 1, 1, 1, 0, 0, 1},
				expectedError:    nil,
			},
			{
				name:             "2. If the set key does not exist, return an array of zeroes as long as the list of members",
				presetValue:      nil,
				key:              "SmismemberKey2",
				command:          []string{"SMISMEMBER", "SmismemberKey2", "one", "two", "three", "four"},
				expectedResponse: []int{0, 0, 0, 0},
				expectedError:    nil,
			},
			{
				name:             "3. Throw error when trying to assert membership when the key does not hold a valid set",
				presetValue:      "Default value",
				key:              "SmismemberKey3",
				command:          []string{"SMISMEMBER", "SmismemberKey3", "one"},
				expectedResponse: nil,
				expectedError:    errors.New("value at key SmismemberKey3 is not a set"),
			},
			{
				name:             "4. Command too short",
				presetValue:      nil,
				key:              "SmismemberKey4",
				command:          []string{"SMISMEMBER", "SmismemberKey4"},
				expectedResponse: nil,
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
					case *set.Set:
						command = []resp.Value{resp.StringValue("SADD"), resp.StringValue(test.key)}
						for _, element := range test.presetValue.(*set.Set).GetAll() {
							command = append(command, []resp.Value{resp.StringValue(element)}...)
						}
						expected = strconv.Itoa(test.presetValue.(*set.Set).Cardinality())
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

				if len(res.Array()) != len(test.expectedResponse) {
					t.Errorf("expected response array of length \"%d\", got \"%d\"",
						len(test.expectedResponse), len(res.Array()))
				}

				for _, item := range res.Array() {
					if !slices.Contains(test.expectedResponse, item.Integer()) {
						t.Errorf("unexpected element \"%d\" in response", item.Integer())
					}
				}
			})
		}
	})

	t.Run("Test_HandleSMOVE", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error()
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			presetValues     map[string]interface{}
			command          []string
			expectedValues   map[string]interface{}
			expectedResponse int
			expectedError    error
		}{
			{
				name: "1. Return 1 after a successful move of a member from source set to destination set",
				presetValues: map[string]interface{}{
					"SmoveSource1":      set.NewSet([]string{"one", "two", "three", "four"}),
					"SmoveDestination1": set.NewSet([]string{"five", "six", "seven", "eight"}),
				},
				command: []string{"SMOVE", "SmoveSource1", "SmoveDestination1", "four"},
				expectedValues: map[string]interface{}{
					"SmoveSource1":      set.NewSet([]string{"one", "two", "three"}),
					"SmoveDestination1": set.NewSet([]string{"four", "five", "six", "seven", "eight"}),
				},
				expectedResponse: 1,
				expectedError:    nil,
			},
			{
				name: "2. Return 0 when trying to move a member from source set to destination set when it doesn't exist in source",
				presetValues: map[string]interface{}{
					"SmoveSource2":      set.NewSet([]string{"one", "two", "three", "four", "five"}),
					"SmoveDestination2": set.NewSet([]string{"five", "six", "seven", "eight"}),
				},
				command: []string{"SMOVE", "SmoveSource2", "SmoveDestination2", "six"},
				expectedValues: map[string]interface{}{
					"SmoveSource2":      set.NewSet([]string{"one", "two", "three", "four", "five"}),
					"SmoveDestination2": set.NewSet([]string{"five", "six", "seven", "eight"}),
				},
				expectedResponse: 0,
				expectedError:    nil,
			},
			{
				name: "3. Return error when the source key is not a set",
				presetValues: map[string]interface{}{
					"SmoveSource3":      "Default value",
					"SmoveDestination3": set.NewSet([]string{"five", "six", "seven", "eight"}),
				},
				command: []string{"SMOVE", "SmoveSource3", "SmoveDestination3", "five"},
				expectedValues: map[string]interface{}{
					"SmoveSource3":      "Default value",
					"SmoveDestination3": set.NewSet([]string{"five", "six", "seven", "eight"}),
				},
				expectedResponse: 0,
				expectedError:    errors.New("source is not a set"),
			},
			{
				name: "4. Return error when the destination key is not a set",
				presetValues: map[string]interface{}{
					"SmoveSource4":      set.NewSet([]string{"one", "two", "three", "four", "five"}),
					"SmoveDestination4": "Default value",
				},
				command: []string{"SMOVE", "SmoveSource4", "SmoveDestination4", "five"},
				expectedValues: map[string]interface{}{
					"SmoveSource4":      set.NewSet([]string{"one", "two", "three", "four", "five"}),
					"SmoveDestination4": "Default value",
				},
				expectedResponse: 0,
				expectedError:    errors.New("destination is not a set"),
			},
			{
				name:          "5. Command too short",
				presetValues:  nil,
				command:       []string{"SMOVE", "SmoveSource5", "SmoveSource6"},
				expectedError: errors.New(constants.WrongArgsResponse),
			},
			{
				name:          "6. Command too long",
				presetValues:  nil,
				command:       []string{"SMOVE", "SmoveSource5", "SmoveSource6", "member1", "member2"},
				expectedError: errors.New(constants.WrongArgsResponse),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValues != nil {
					var command []resp.Value
					var expected string
					for key, value := range test.presetValues {
						switch value.(type) {
						case string:
							command = []resp.Value{
								resp.StringValue("SET"),
								resp.StringValue(key),
								resp.StringValue(value.(string)),
							}
							expected = "ok"
						case *set.Set:
							command = []resp.Value{resp.StringValue("SADD"), resp.StringValue(key)}
							for _, element := range value.(*set.Set).GetAll() {
								command = append(command, []resp.Value{resp.StringValue(element)}...)
							}
							expected = strconv.Itoa(value.(*set.Set).Cardinality())
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

				// Check if the resulting set(s) contain the expected members.
				if test.expectedValues == nil {
					return
				}

				for key, value := range test.expectedValues {
					switch value.(type) {
					case string:
						if err := client.WriteArray([]resp.Value{resp.StringValue("GET"), resp.StringValue(key)}); err != nil {
							t.Error(err)
						}
						res, _, err = client.ReadValue()
						if err != nil {
							t.Error(err)
						}
						if res.String() != value.(string) {
							t.Errorf("expected value at key \"%s\" to be \"%s\", got \"%s\"", key, value.(string), res.String())
						}
					case *set.Set:
						if err := client.WriteArray([]resp.Value{
							resp.StringValue("SMEMBERS"),
							resp.StringValue(key),
						}); err != nil {
							t.Error(err)
						}
						res, _, err = client.ReadValue()
						if err != nil {
							t.Error(err)
						}

						if len(res.Array()) != value.(*set.Set).Cardinality() {
							t.Errorf("expected set at key \"%s\" to have cardinality %d, got %d",
								key, value.(*set.Set).Cardinality(), len(res.Array()))
						}

						for _, item := range res.Array() {
							if !value.(*set.Set).Contains(item.String()) {
								t.Errorf("unexpected memeber \"%s\", in response", item.String())
							}
						}
					}
				}
			})
		}
	})

	t.Run("Test_HandleSPOP", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error()
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
			expectedValue    int // The final cardinality of the resulting set
			expectedResponse []string
			expectedError    error
		}{
			{
				name:             "1. Return multiple popped elements and modify the set",
				key:              "SpopKey1",
				presetValue:      set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven", "eight"}),
				command:          []string{"SPOP", "SpopKey1", "3"},
				expectedValue:    5,
				expectedResponse: []string{"one", "two", "three", "four", "five", "six", "seven", "eight"},
				expectedError:    nil,
			},
			{
				name:             "2. Return error when the source key is not a set",
				key:              "SpopKey2",
				presetValue:      "Default value",
				command:          []string{"SPOP", "SpopKey2"},
				expectedValue:    0,
				expectedResponse: nil,
				expectedError:    errors.New("value at SpopKey2 is not a set"),
			},
			{
				name:             "3. Command too short",
				presetValue:      nil,
				command:          []string{"SPOP"},
				expectedValue:    0,
				expectedResponse: nil,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:          "4. Command too long",
				presetValue:   nil,
				command:       []string{"SPOP", "SpopSource5", "SpopSource6", "member1", "member2"},
				expectedValue: 0,
				expectedError: errors.New(constants.WrongArgsResponse),
			},
			{
				name:          "5. Throw error when count is not an integer",
				presetValue:   nil,
				command:       []string{"SPOP", "SpopKey1", "count"},
				expectedValue: 0,
				expectedError: errors.New("count must be an integer"),
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
					case *set.Set:
						command = []resp.Value{resp.StringValue("SADD"), resp.StringValue(test.key)}
						for _, element := range test.presetValue.(*set.Set).GetAll() {
							command = append(command, []resp.Value{resp.StringValue(element)}...)
						}
						expected = strconv.Itoa(test.presetValue.(*set.Set).Cardinality())
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

				// Check that each returned element is in the list of expected elements.
				for _, item := range res.Array() {
					if !slices.Contains(test.expectedResponse, item.String()) {
						t.Errorf("unexpected element \"%s\" in response", item.String())
					}
				}

				// Check if the resulting set's cardinality is as expected.
				if err := client.WriteArray([]resp.Value{resp.StringValue("SCARD"), resp.StringValue(test.key)}); err != nil {
					t.Error(err)
				}
				res, _, err = client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if res.Integer() != test.expectedValue {
					t.Errorf("expected set at key \"%s\" to have cardinality %d, got %d",
						test.key, test.expectedValue, res.Integer())
				}
			})
		}
	})

	t.Run("Test_HandleSRANDMEMBER", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error()
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
			expectedValue    int // The final cardinality of the resulting set
			allowRepeat      bool
			expectedResponse []string
			expectedError    error
		}{
			{
				// 1. Return multiple random elements without removing them
				// Count is positive, do not allow repeated elements
				name:             "1. Return multiple random elements without removing them",
				key:              "SRandMemberKey1",
				presetValue:      set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven", "eight"}),
				command:          []string{"SRANDMEMBER", "SRandMemberKey1", "3"},
				expectedValue:    8,
				allowRepeat:      false,
				expectedResponse: []string{"one", "two", "three", "four", "five", "six", "seven", "eight"},
				expectedError:    nil,
			},
			{
				// 2. Return multiple random elements without removing them
				// Count is negative, so allow repeated numbers
				name:             "2. Return multiple random elements without removing them",
				key:              "SRandMemberKey2",
				presetValue:      set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven", "eight"}),
				command:          []string{"SRANDMEMBER", "SRandMemberKey2", "-5"},
				expectedValue:    8,
				allowRepeat:      true,
				expectedResponse: []string{"one", "two", "three", "four", "five", "six", "seven", "eight"},
				expectedError:    nil,
			},
			{
				name:             "3. Return error when the source key is not a set",
				key:              "SRandMemberKey3",
				presetValue:      "Default value",
				command:          []string{"SRANDMEMBER", "SRandMemberKey3"},
				expectedValue:    0,
				expectedResponse: []string{},
				expectedError:    errors.New("value at SRandMemberKey3 is not a set"),
			},
			{
				name:          "4. Command too short",
				command:       []string{"SRANDMEMBER"},
				expectedError: errors.New(constants.WrongArgsResponse),
			},
			{
				name:          "5. Command too long",
				command:       []string{"SRANDMEMBER", "SRandMemberSource5", "SRandMemberSource6", "member1", "member2"},
				expectedError: errors.New(constants.WrongArgsResponse),
			},
			{
				name:          "6. Throw error when count is not an integer",
				command:       []string{"SRANDMEMBER", "SRandMemberKey1", "count"},
				expectedError: errors.New("count must be an integer"),
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
					case *set.Set:
						command = []resp.Value{resp.StringValue("SADD"), resp.StringValue(test.key)}
						for _, element := range test.presetValue.(*set.Set).GetAll() {
							command = append(command, []resp.Value{resp.StringValue(element)}...)
						}
						expected = strconv.Itoa(test.presetValue.(*set.Set).Cardinality())
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

				// Check that each returned element is in the list of expected elements.
				for _, item := range res.Array() {
					if !slices.Contains(test.expectedResponse, item.String()) {
						t.Errorf("unexpected element \"%s\" in response", item.String())
					}
				}

				// If no repeats are allowed, check if the response contains any repeated elements
				if !test.allowRepeat {
					s := set.NewSet(func() []string {
						elements := make([]string, len(res.Array()))
						for i, item := range res.Array() {
							elements[i] = item.String()
						}
						return elements
					}())
					if s.Cardinality() != len(res.Array()) {
						t.Error("response has repeated elements, expected only unique elements.")
					}
				}

				// Check if the resulting set's cardinality is as expected.
				if err := client.WriteArray([]resp.Value{resp.StringValue("SCARD"), resp.StringValue(test.key)}); err != nil {
					t.Error(err)
				}
				res, _, err = client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if res.Integer() != test.expectedValue {
					t.Errorf("expected set at key \"%s\" to have cardinality %d, got %d",
						test.key, test.expectedValue, res.Integer())
				}
			})
		}
	})

	t.Run("Test_HandleSREM", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error()
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
			expectedValue    *set.Set // The final cardinality of the resulting set
			expectedResponse int
			expectedError    error
		}{
			{
				name:             "1. Remove multiple elements and return the number of elements removed",
				key:              "SremKey1",
				presetValue:      set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven", "eight"}),
				command:          []string{"SREM", "SremKey1", "one", "two", "three", "nine"},
				expectedValue:    set.NewSet([]string{"four", "five", "six", "seven", "eight"}),
				expectedResponse: 3,
				expectedError:    nil,
			},
			{
				name:             "2. If key does not exist, return 0",
				key:              "SremKey2",
				presetValue:      nil,
				command:          []string{"SREM", "SremKey1", "one", "two", "three", "nine"},
				expectedValue:    nil,
				expectedResponse: 0,
				expectedError:    nil,
			},
			{
				name:             "3. Return error when the source key is not a set",
				key:              "SremKey3",
				presetValue:      "Default value",
				command:          []string{"SREM", "SremKey3", "one"},
				expectedValue:    nil,
				expectedResponse: 0,
				expectedError:    errors.New("value at key SremKey3 is not a set"),
			},
			{
				name:          "4. Command too short",
				command:       []string{"SREM", "SremKey"},
				expectedError: errors.New(constants.WrongArgsResponse),
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
					case *set.Set:
						command = []resp.Value{resp.StringValue("SADD"), resp.StringValue(test.key)}
						for _, element := range test.presetValue.(*set.Set).GetAll() {
							command = append(command, []resp.Value{resp.StringValue(element)}...)
						}
						expected = strconv.Itoa(test.presetValue.(*set.Set).Cardinality())
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

				// Check if the resulting set(s) contain the expected members.
				if test.expectedValue == nil {
					return
				}

				if err := client.WriteArray([]resp.Value{resp.StringValue("SMEMBERS"), resp.StringValue(test.key)}); err != nil {
					t.Error(err)
				}
				res, _, err = client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if len(res.Array()) != test.expectedValue.Cardinality() {
					t.Errorf("expected set at key \"%s\" to have cardinality %d, got %d",
						test.key, test.expectedValue.Cardinality(), len(res.Array()))
				}

				for _, item := range res.Array() {
					if !test.expectedValue.Contains(item.String()) {
						t.Errorf("unexpected memeber \"%s\", in response", item.String())
					}
				}
			})
		}
	})

	t.Run("Test_HandleSUNION", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error()
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			presetValues     map[string]interface{}
			command          []string
			expectedResponse []string
			expectedError    error
		}{
			{
				name: "1. Get the union between 2 sets.",
				presetValues: map[string]interface{}{
					"SunionKey1": set.NewSet([]string{"one", "two", "three", "four", "five"}),
					"SunionKey2": set.NewSet([]string{"three", "four", "five", "six", "seven", "eight"}),
				},
				command:          []string{"SUNION", "SunionKey1", "SunionKey2"},
				expectedResponse: []string{"one", "two", "three", "four", "five", "six", "seven", "eight"},
				expectedError:    nil,
			},
			{
				name: "2. Get the union between 3 sets.",
				presetValues: map[string]interface{}{
					"SunionKey3": set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven", "eight"}),
					"SunionKey4": set.NewSet([]string{"one", "two", "thirty-six", "twelve", "eleven", "eight"}),
					"SunionKey5": set.NewSet([]string{"one", "eight", "nine", "ten", "twelve"}),
				},
				command: []string{"SUNION", "SunionKey3", "SunionKey4", "SunionKey5"},
				expectedResponse: []string{
					"one", "two", "three", "four", "five", "six", "seven", "eight", "nine",
					"ten", "eleven", "twelve", "thirty-six",
				},
				expectedError: nil,
			},
			{
				name: "3. Throw an error if any of the provided keys are not sets",
				presetValues: map[string]interface{}{
					"SunionKey6": set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven", "eight"}),
					"SunionKey7": "Default value",
					"SunionKey8": set.NewSet([]string{"one"}),
				},
				command:          []string{"SUNION", "SunionKey6", "SunionKey7", "SunionKey8"},
				expectedResponse: nil,
				expectedError:    errors.New("value at key SunionKey7 is not a set"),
			},
			{
				name: "4. Throw error any of the keys does not hold a set.",
				presetValues: map[string]interface{}{
					"SunionKey9":  "Default value",
					"SunionKey10": set.NewSet([]string{"one", "two", "thirty-six", "twelve", "eleven"}),
					"SunionKey11": set.NewSet([]string{"seven", "eight", "nine", "ten", "twelve"}),
				},
				command:          []string{"SUNION", "SunionKey9", "SunionKey10", "SunionKey11"},
				expectedResponse: nil,
				expectedError:    errors.New("value at key SunionKey9 is not a set"),
			},
			{
				name:             "6. Command too short",
				command:          []string{"SUNION"},
				expectedResponse: []string{},
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValues != nil {
					var command []resp.Value
					var expected string
					for key, value := range test.presetValues {
						switch value.(type) {
						case string:
							command = []resp.Value{
								resp.StringValue("SET"),
								resp.StringValue(key),
								resp.StringValue(value.(string)),
							}
							expected = "ok"
						case *set.Set:
							command = []resp.Value{resp.StringValue("SADD"), resp.StringValue(key)}
							for _, element := range value.(*set.Set).GetAll() {
								command = append(command, []resp.Value{resp.StringValue(element)}...)
							}
							expected = strconv.Itoa(value.(*set.Set).Cardinality())
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

				if len(res.Array()) != len(test.expectedResponse) {
					t.Errorf("expected response array of length \"%d\", got \"%d\"",
						len(test.expectedResponse), len(res.Array()))
				}

				for _, item := range res.Array() {
					if !slices.Contains(test.expectedResponse, item.String()) {
						t.Errorf("unexpected element \"%s\" in response", item.String())
					}
				}
			})
		}
	})

	t.Run("Test_HandleSUNIONSTORE", func(t *testing.T) {
		t.Parallel()
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error()
			return
		}
		defer func() {
			_ = conn.Close()
		}()
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			presetValues     map[string]interface{}
			destination      string
			command          []string
			expectedValue    *set.Set
			expectedResponse int
			expectedError    error
		}{
			{
				name: "1. Get the intersection between 2 sets and store it at the destination.",
				presetValues: map[string]interface{}{
					"SunionStoreKey1": set.NewSet([]string{"one", "two", "three", "four", "five"}),
					"SunionStoreKey2": set.NewSet([]string{"three", "four", "five", "six", "seven", "eight"}),
				},
				destination:      "SunionStoreDestination1",
				command:          []string{"SUNIONSTORE", "SunionStoreDestination1", "SunionStoreKey1", "SunionStoreKey2"},
				expectedValue:    set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven", "eight"}),
				expectedResponse: 8,
				expectedError:    nil,
			},
			{
				name: "2. Get the intersection between 3 sets and store it at the destination key.",
				presetValues: map[string]interface{}{
					"SunionStoreKey3": set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven", "eight"}),
					"SunionStoreKey4": set.NewSet([]string{"one", "two", "thirty-six", "twelve", "eleven", "eight"}),
					"SunionStoreKey5": set.NewSet([]string{"one", "seven", "eight", "nine", "ten", "twelve"}),
				},
				destination: "SunionStoreDestination2",
				command:     []string{"SUNIONSTORE", "SunionStoreDestination2", "SunionStoreKey3", "SunionStoreKey4", "SunionStoreKey5"},
				expectedValue: set.NewSet([]string{
					"one", "two", "three", "four", "five", "six", "seven", "eight",
					"nine", "ten", "eleven", "twelve", "thirty-six",
				}),
				expectedResponse: 13,
				expectedError:    nil,
			},
			{
				name: "3. Throw error when any of the keys is not a set",
				presetValues: map[string]interface{}{
					"SunionStoreKey6": set.NewSet([]string{"one", "two", "three", "four", "five", "six", "seven", "eight"}),
					"SunionStoreKey7": "Default value",
					"SunionStoreKey8": set.NewSet([]string{"one"}),
				},
				destination:      "SunionStoreDestination3",
				command:          []string{"SUNIONSTORE", "SunionStoreDestination3", "SunionStoreKey6", "SunionStoreKey7", "SunionStoreKey8"},
				expectedValue:    nil,
				expectedResponse: 0,
				expectedError:    errors.New("value at key SunionStoreKey7 is not a set"),
			},
			{
				name:             "5. Command too short",
				command:          []string{"SUNIONSTORE", "SunionStoreDestination6"},
				expectedResponse: 0,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValues != nil {
					var command []resp.Value
					var expected string
					for key, value := range test.presetValues {
						switch value.(type) {
						case string:
							command = []resp.Value{
								resp.StringValue("SET"),
								resp.StringValue(key),
								resp.StringValue(value.(string)),
							}
							expected = "ok"
						case *set.Set:
							command = []resp.Value{resp.StringValue("SADD"), resp.StringValue(key)}
							for _, element := range value.(*set.Set).GetAll() {
								command = append(command, []resp.Value{resp.StringValue(element)}...)
							}
							expected = strconv.Itoa(value.(*set.Set).Cardinality())
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

				// Check if the resulting set(s) contain the expected members.
				if test.expectedValue == nil {
					return
				}

				if err := client.WriteArray([]resp.Value{
					resp.StringValue("SMEMBERS"),
					resp.StringValue(test.destination),
				}); err != nil {
					t.Error(err)
				}
				res, _, err = client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if len(res.Array()) != test.expectedValue.Cardinality() {
					t.Errorf("expected set at key \"%s\" to have cardinality %d, got %d",
						test.destination, test.expectedValue.Cardinality(), len(res.Array()))
				}

				for _, item := range res.Array() {
					if !test.expectedValue.Contains(item.String()) {
						t.Errorf("unexpected memeber \"%s\", in response", item.String())
					}
				}
			})
		}
	})
}
