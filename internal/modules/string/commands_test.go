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

package str_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/echovault/sugardb/internal"
	"github.com/echovault/sugardb/internal/constants"
	"github.com/echovault/sugardb/pkg/config"
	"github.com/echovault/sugardb/sugardb"
	"github.com/tidwall/resp"
)

func Test_String(t *testing.T) {
	port, err := internal.GetFreePort()
	if err != nil {
		t.Error()
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

	t.Run("Test_HandleSetRange", func(t *testing.T) {
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
			presetValue      string
			command          []string
			expectedValue    string
			expectedResponse int
			expectedError    error
		}{
			{
				name:             "Test that SETRANGE on non-existent string creates new string",
				key:              "SetRangeKey1",
				presetValue:      "",
				command:          []string{"SETRANGE", "SetRangeKey1", "10", "New String Value"},
				expectedValue:    "New String Value",
				expectedResponse: len("New String Value"),
				expectedError:    nil,
			},
			{
				name:             "Test SETRANGE with an offset that leads to a longer resulting string",
				key:              "SetRangeKey2",
				presetValue:      "Original String Value",
				command:          []string{"SETRANGE", "SetRangeKey2", "16", "Portion Replaced With This New String"},
				expectedValue:    "Original String Portion Replaced With This New String",
				expectedResponse: len("Original String Portion Replaced With This New String"),
				expectedError:    nil,
			},
			{
				name:             "SETRANGE with negative offset prepends the string",
				key:              "SetRangeKey3",
				presetValue:      "This is a preset value",
				command:          []string{"SETRANGE", "SetRangeKey3", "-10", "Prepended "},
				expectedValue:    "Prepended This is a preset value",
				expectedResponse: len("Prepended This is a preset value"),
				expectedError:    nil,
			},
			{
				name:             "SETRANGE with offset that embeds new string inside the old string",
				key:              "SetRangeKey4",
				presetValue:      "This is a preset value",
				command:          []string{"SETRANGE", "SetRangeKey4", "0", "That"},
				expectedValue:    "That is a preset value",
				expectedResponse: len("That is a preset value"),
				expectedError:    nil,
			},
			{
				name:             "SETRANGE with offset longer than original lengths appends the string",
				key:              "SetRangeKey5",
				presetValue:      "This is a preset value",
				command:          []string{"SETRANGE", "SetRangeKey5", "100", " Appended"},
				expectedValue:    "This is a preset value Appended",
				expectedResponse: len("This is a preset value Appended"),
				expectedError:    nil,
			},
			{
				name:             "SETRANGE with offset on the last character replaces last character with new string",
				key:              "SetRangeKey6",
				presetValue:      "This is a preset value",
				command:          []string{"SETRANGE", "SetRangeKey6", strconv.Itoa(len("This is a preset value") - 1), " replaced"},
				expectedValue:    "This is a preset valu replaced",
				expectedResponse: len("This is a preset valu replaced"),
				expectedError:    nil,
			},
			{
				name:             " Offset not integer",
				command:          []string{"SETRANGE", "key", "offset", "value"},
				expectedResponse: 0,
				expectedError:    errors.New("offset must be an integer"),
			},
			{
				name:             "SETRANGE target is not a string",
				key:              "test-int",
				presetValue:      "10",
				command:          []string{"SETRANGE", "test-int", "10", "value"},
				expectedResponse: 0,
				expectedError:    errors.New("value at key test-int is not a string"),
			},
			{
				name:             "Command too short",
				command:          []string{"SETRANGE", "key"},
				expectedResponse: 0,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "Command too long",
				command:          []string{"SETRANGE", "key", "offset", "value", "value1"},
				expectedResponse: 0,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValue != "" {
					if err = client.WriteArray([]resp.Value{
						resp.StringValue("SET"),
						resp.StringValue(test.key),
						resp.StringValue(test.presetValue),
					}); err != nil {
						t.Error(err)
					}
					res, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if !strings.EqualFold(res.String(), "ok") {
						t.Errorf("expected preset response to be OK, got %s", res.String())
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

	t.Run("Test_HandleStrLen", func(t *testing.T) {
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
			presetValue      string
			command          []string
			expectedResponse int
			expectedError    error
		}{
			{
				name:             "Return the correct string length for an existing string",
				key:              "StrLenKey1",
				presetValue:      "Test String",
				command:          []string{"STRLEN", "StrLenKey1"},
				expectedResponse: len("Test String"),
				expectedError:    nil,
			},
			{
				name:             "If the string does not exist, return 0",
				key:              "StrLenKey2",
				presetValue:      "",
				command:          []string{"STRLEN", "StrLenKey2"},
				expectedResponse: 0,
				expectedError:    nil,
			},
			{
				name:             "Too few args",
				key:              "StrLenKey3",
				presetValue:      "",
				command:          []string{"STRLEN"},
				expectedResponse: 0,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "Too many args",
				key:              "StrLenKey4",
				presetValue:      "",
				command:          []string{"STRLEN", "StrLenKey4", "StrLenKey5"},
				expectedResponse: 0,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValue != "" {
					if err = client.WriteArray([]resp.Value{
						resp.StringValue("SET"),
						resp.StringValue(test.key),
						resp.StringValue(test.presetValue),
					}); err != nil {
						t.Error(err)
					}
					res, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if !strings.EqualFold(res.String(), "ok") {
						t.Errorf("expected preset response to be OK, got %s", res.String())
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

	t.Run("Test_HandleSubStr", func(t *testing.T) {
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
			presetValue      string
			command          []string
			expectedResponse string
			expectedError    error
		}{
			{
				name:             "Return substring within the range of the string",
				key:              "SubStrKey1",
				presetValue:      "Test String One",
				command:          []string{"SUBSTR", "SubStrKey1", "5", "10"},
				expectedResponse: "String",
				expectedError:    nil,
			},
			{
				name:             "Return substring at the end of the string with exact end index",
				key:              "SubStrKey2",
				presetValue:      "Test String Two",
				command:          []string{"SUBSTR", "SubStrKey2", "12", "14"},
				expectedResponse: "Two",
				expectedError:    nil,
			},
			{
				name:             "Return substring at the end of the string with end index greater than length",
				key:              "SubStrKey3",
				presetValue:      "Test String Three",
				command:          []string{"SUBSTR", "SubStrKey3", "12", "75"},
				expectedResponse: "Three",
				expectedError:    nil,
			},
			{
				name:             "Return the substring at the start of the string with 0 start index",
				key:              "SubStrKey4",
				presetValue:      "Test String Four",
				command:          []string{"SUBSTR", "SubStrKey4", "0", "3"},
				expectedResponse: "Test",
				expectedError:    nil,
			},
			{
				// Return the substring with negative start index.
				// Substring should begin abs(start) from the end of the string when start is negative.
				name:             "Return the substring with negative start index",
				key:              "SubStrKey5",
				presetValue:      "Test String Five",
				command:          []string{"SUBSTR", "SubStrKey5", "-11", "10"},
				expectedResponse: "String",
				expectedError:    nil,
			},
			{
				// Return reverse substring with end index smaller than start index.
				// When end index is smaller than start index, the 2 indices are reversed.
				name:             "Return reverse substring with end index smaller than start index",
				key:              "SubStrKey6",
				presetValue:      "Test String Six",
				command:          []string{"SUBSTR", "SubStrKey6", "4", "0"},
				expectedResponse: "tseT",
				expectedError:    nil,
			},
			{
				name:          "Command too short",
				command:       []string{"SUBSTR", "key", "10"},
				expectedError: errors.New(constants.WrongArgsResponse),
			},
			{
				name:          "Command too long",
				command:       []string{"SUBSTR", "key", "10", "15", "20"},
				expectedError: errors.New(constants.WrongArgsResponse),
			},
			{
				name:          "Start index is not an integer",
				command:       []string{"SUBSTR", "key", "start", "10"},
				expectedError: errors.New("start and end indices must be integers"),
			},
			{
				name:          "End index is not an integer",
				command:       []string{"SUBSTR", "key", "0", "end"},
				expectedError: errors.New("start and end indices must be integers"),
			},
			{
				name:          "Non-existent key",
				command:       []string{"SUBSTR", "non-existent-key", "0", "10"},
				expectedError: errors.New("key non-existent-key does not exist"),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValue != "" {
					if err = client.WriteArray([]resp.Value{
						resp.StringValue("SET"),
						resp.StringValue(test.key),
						resp.StringValue(test.presetValue),
					}); err != nil {
						t.Error(err)
					}
					res, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if !strings.EqualFold(res.String(), "ok") {
						t.Errorf("expected preset response to be OK, got %s", res.String())
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
			})
		}
	})

	t.Run("Test_HandleAppend", func(t *testing.T) {
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
			expectedError    error
		}{
			{
				name:             "Test APPEND with no preset value",
				key:              "AppendKey1",
				command:          []string{"APPEND", "AppendKey1", "Hello"},
				expectedResponse: 5,
				expectedError:    nil,
			},
			{
				name:             "Test APPEND with preset value",
				key:              "AppendKey2",
				presetValue:      "Hello ",
				command:          []string{"APPEND", "AppendKey2", "World"},
				expectedResponse: 11,
				expectedError:    nil,
			},
			{
				name:             "Test APPEND with integer preset value",
				key:              "AppendKey4",
				presetValue:      10,
				command:          []string{"APPEND", "AppendKey4", "World"},
				expectedResponse: 0,
				expectedError:    errors.New("Value at key AppendKey4 is not a string"),
			},
			{
				name:          "Command too short",
				command:       []string{"APPEND", "AppendKey5"},
				expectedError: errors.New(constants.WrongArgsResponse),
			},
			{
				name:          "Command too long",
				command:       []string{"APPEND", "AppendKey5", "new value", "extra value"},
				expectedError: errors.New(constants.WrongArgsResponse),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValue != "" {
					if err = client.WriteArray([]resp.Value{
						resp.StringValue("SET"),
						resp.StringValue(test.key),
						resp.AnyValue(test.presetValue),
					}); err != nil {
						t.Error(err)
					}
					res, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if !strings.EqualFold(res.String(), "ok") {
						t.Errorf("expected preset response to be OK, got %s", res.String())
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
}
