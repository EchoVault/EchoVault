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

package sorted_set_test

import (
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/echovault/sugardb/internal"
	"github.com/echovault/sugardb/internal/constants"
	"github.com/echovault/sugardb/internal/modules/sorted_set"
	"github.com/echovault/sugardb/pkg/config"
	"github.com/echovault/sugardb/sugardb"
	"github.com/tidwall/resp"
)

func Test_SortedSet(t *testing.T) {
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

	t.Run("Test_HandleZADD", func(t *testing.T) {
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
			presetValue      *sorted_set.SortedSet
			key              string
			command          []string
			expectedResponse int
			expectedError    error
		}{
			{
				name:             "1. Create new sorted set and return the cardinality of the new sorted set",
				presetValue:      nil,
				key:              "ZaddKey1",
				command:          []string{"ZADD", "ZaddKey1", "5.5", "member1", "67.77", "member2", "10", "member3", "-inf", "member4", "+inf", "member5"},
				expectedResponse: 5,
				expectedError:    nil,
			},
			{
				name: "2. Only add the elements that do not currently exist in the sorted set when NX flag is provided",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "member1", Score: sorted_set.Score(5.5)},
					{Value: "member2", Score: sorted_set.Score(67.77)},
					{Value: "member3", Score: sorted_set.Score(10)},
				}),
				key:              "ZaddKey2",
				command:          []string{"ZADD", "ZaddKey2", "NX", "5.5", "member1", "67.77", "member4", "10", "member5"},
				expectedResponse: 2,
				expectedError:    nil,
			},
			{
				name: "3. Do not add any elements when providing existing members with NX flag",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "member1", Score: sorted_set.Score(5.5)},
					{Value: "member2", Score: sorted_set.Score(67.77)},
					{Value: "member3", Score: sorted_set.Score(10)},
				}),
				key:              "ZaddKey3",
				command:          []string{"ZADD", "ZaddKey3", "NX", "5.5", "member1", "67.77", "member2", "10", "member3"},
				expectedResponse: 0,
				expectedError:    nil,
			},
			{
				name: "4. Successfully add elements to an existing set when XX flag is provided with existing elements",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "member1", Score: sorted_set.Score(5.5)},
					{Value: "member2", Score: sorted_set.Score(67.77)},
					{Value: "member3", Score: sorted_set.Score(10)},
				}),
				key:              "ZaddKey4",
				command:          []string{"ZADD", "ZaddKey4", "XX", "CH", "55", "member1", "1005", "member2", "15", "member3", "99.75", "member4"},
				expectedResponse: 3,
				expectedError:    nil,
			},
			{
				name: "5. Fail to add element when providing XX flag with elements that do not exist in the sorted set.",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "member1", Score: sorted_set.Score(5.5)},
					{Value: "member2", Score: sorted_set.Score(67.77)},
					{Value: "member3", Score: sorted_set.Score(10)},
				}),
				key:              "ZaddKey5",
				command:          []string{"ZADD", "ZaddKey5", "XX", "5.5", "member4", "100.5", "member5", "15", "member6"},
				expectedResponse: 0,
				expectedError:    nil,
			},
			{
				// 6. Only update the elements where provided score is greater than current score and GT flag is provided
				// Return only the new elements added by default
				name: "6. Only update the elements where provided score is greater than current score and GT flag is provided",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "member1", Score: sorted_set.Score(5.5)},
					{Value: "member2", Score: sorted_set.Score(67.77)},
					{Value: "member3", Score: sorted_set.Score(10)},
				}),
				key:              "ZaddKey6",
				command:          []string{"ZADD", "ZaddKey6", "XX", "CH", "GT", "7.5", "member1", "100.5", "member4", "15", "member5"},
				expectedResponse: 1,
				expectedError:    nil,
			},
			{
				// 7. Only update the elements where provided score is less than current score if LT flag is provided
				// Return only the new elements added by default.
				name: "7. Only update the elements where provided score is less than current score if LT flag is provided",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "member1", Score: sorted_set.Score(5.5)},
					{Value: "member2", Score: sorted_set.Score(67.77)},
					{Value: "member3", Score: sorted_set.Score(10)},
				}),
				key:              "ZaddKey7",
				command:          []string{"ZADD", "ZaddKey7", "XX", "LT", "3.5", "member1", "100.5", "member4", "15", "member5"},
				expectedResponse: 0,
				expectedError:    nil,
			},
			{
				name: "8. Return all the elements that were updated AND added when CH flag is provided",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "member1", Score: sorted_set.Score(5.5)},
					{Value: "member2", Score: sorted_set.Score(67.77)},
					{Value: "member3", Score: sorted_set.Score(10)},
				}),
				key:              "ZaddKey8",
				command:          []string{"ZADD", "ZaddKey8", "XX", "LT", "CH", "3.5", "member1", "100.5", "member4", "15", "member5"},
				expectedResponse: 1,
				expectedError:    nil,
			},
			{
				name: "9. Increment the member by score",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "member1", Score: sorted_set.Score(5.5)},
					{Value: "member2", Score: sorted_set.Score(67.77)},
					{Value: "member3", Score: sorted_set.Score(10)},
				}),
				key:              "ZaddKey9",
				command:          []string{"ZADD", "ZaddKey9", "INCR", "5.5", "member3"},
				expectedResponse: 0,
				expectedError:    nil,
			},
			{
				name:             "10. Fail when GT/LT flag is provided alongside NX flag",
				presetValue:      nil,
				key:              "ZaddKey10",
				command:          []string{"ZADD", "ZaddKey10", "NX", "LT", "CH", "3.5", "member1", "100.5", "member4", "15", "member5"},
				expectedResponse: 0,
				expectedError:    errors.New("GT/LT flags not allowed if NX flag is provided"),
			},
			{
				name:             "11. Command is too short",
				presetValue:      nil,
				key:              "ZaddKey11",
				command:          []string{"ZADD", "ZaddKey11"},
				expectedResponse: 0,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "12. Throw error when score/member entries are do not match",
				presetValue:      nil,
				key:              "ZaddKey11",
				command:          []string{"ZADD", "ZaddKey12", "10.5", "member1", "12.5"},
				expectedResponse: 0,
				expectedError:    errors.New("score/member pairs must be float/string"),
			},
			{
				name:             "13. Throw error when INCR flag is passed with more than one score/member pair",
				presetValue:      nil,
				key:              "ZaddKey13",
				command:          []string{"ZADD", "ZaddKey13", "INCR", "10.5", "member1", "12.5", "member2"},
				expectedResponse: 0,
				expectedError:    errors.New("cannot pass more than one score/member pair when INCR flag is provided"),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetValue != nil {
					var command []resp.Value
					var expected string

					command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(test.key)}
					for _, member := range test.presetValue.GetAll() {
						command = append(command, []resp.Value{
							resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
							resp.StringValue(string(member.Value)),
						}...)
					}

					if err = client.WriteArray(command); err != nil {
						t.Error(err)
					}
					res, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if res.Integer() != test.presetValue.Cardinality() {
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

	t.Run("Test_HandleZCARD", func(t *testing.T) {
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
				name: "1. Get cardinality of valid sorted set.",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "member1", Score: sorted_set.Score(5.5)},
					{Value: "member2", Score: sorted_set.Score(67.77)},
					{Value: "member3", Score: sorted_set.Score(10)},
				}),
				key:              "ZcardKey1",
				command:          []string{"ZCARD", "ZcardKey1"},
				expectedResponse: 3,
				expectedError:    nil,
			},
			{
				name:             "2. Return 0 when trying to get cardinality from non-existent key",
				presetValue:      nil,
				key:              "ZcardKey2",
				command:          []string{"ZCARD", "ZcardKey2"},
				expectedResponse: 0,
				expectedError:    nil,
			},
			{
				name:             "3. Command is too short",
				presetValue:      nil,
				key:              "ZcardKey3",
				command:          []string{"ZCARD"},
				expectedResponse: 0,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "4. Command too long",
				presetValue:      nil,
				key:              "ZcardKey4",
				command:          []string{"ZCARD", "ZcardKey4", "ZcardKey5"},
				expectedResponse: 0,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "5. Return error when not a sorted set",
				presetValue:      "Default value",
				key:              "ZcardKey5",
				command:          []string{"ZCARD", "ZcardKey5"},
				expectedResponse: 0,
				expectedError:    errors.New("value at ZcardKey5 is not a sorted set"),
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
					case *sorted_set.SortedSet:
						command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(test.key)}
						for _, member := range test.presetValue.(*sorted_set.SortedSet).GetAll() {
							command = append(command, []resp.Value{
								resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
								resp.StringValue(string(member.Value)),
							}...)
						}
						expected = strconv.Itoa(test.presetValue.(*sorted_set.SortedSet).Cardinality())
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

	t.Run("Test_HandleZCOUNT", func(t *testing.T) {
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
				name: "1. Get entire count using infinity boundaries",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "member1", Score: sorted_set.Score(5.5)},
					{Value: "member2", Score: sorted_set.Score(67.77)},
					{Value: "member3", Score: sorted_set.Score(10)},
					{Value: "member4", Score: sorted_set.Score(1083.13)},
					{Value: "member5", Score: sorted_set.Score(11)},
					{Value: "member6", Score: sorted_set.Score(math.Inf(-1))},
					{Value: "member7", Score: sorted_set.Score(math.Inf(1))},
				}),
				key:              "ZcountKey1",
				command:          []string{"ZCOUNT", "ZcountKey1", "-inf", "+inf"},
				expectedResponse: 7,
				expectedError:    nil,
			},
			{
				name: "2. Get count of sub-set from -inf to limit",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "member1", Score: sorted_set.Score(5.5)},
					{Value: "member2", Score: sorted_set.Score(67.77)},
					{Value: "member3", Score: sorted_set.Score(10)},
					{Value: "member4", Score: sorted_set.Score(1083.13)},
					{Value: "member5", Score: sorted_set.Score(11)},
					{Value: "member6", Score: sorted_set.Score(math.Inf(-1))},
					{Value: "member7", Score: sorted_set.Score(math.Inf(1))},
				}),
				key:              "ZcountKey2",
				command:          []string{"ZCOUNT", "ZcountKey2", "-inf", "90"},
				expectedResponse: 5,
				expectedError:    nil,
			},
			{
				name: "3. Get count of sub-set from bottom boundary to +inf limit",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "member1", Score: sorted_set.Score(5.5)},
					{Value: "member2", Score: sorted_set.Score(67.77)},
					{Value: "member3", Score: sorted_set.Score(10)},
					{Value: "member4", Score: sorted_set.Score(1083.13)},
					{Value: "member5", Score: sorted_set.Score(11)},
					{Value: "member6", Score: sorted_set.Score(math.Inf(-1))},
					{Value: "member7", Score: sorted_set.Score(math.Inf(1))},
				}),
				key:              "ZcountKey3",
				command:          []string{"ZCOUNT", "ZcountKey3", "1000", "+inf"},
				expectedResponse: 2,
				expectedError:    nil,
			},
			{
				name:             "4. Return error when bottom boundary is not a valid double/float",
				presetValue:      nil,
				key:              "ZcountKey4",
				command:          []string{"ZCOUNT", "ZcountKey4", "min", "10"},
				expectedResponse: 0,
				expectedError:    errors.New("min constraint must be a double"),
			},
			{
				name:             "5. Return error when top boundary is not a valid double/float",
				presetValue:      nil,
				key:              "ZcountKey5",
				command:          []string{"ZCOUNT", "ZcountKey5", "-10", "max"},
				expectedResponse: 0,
				expectedError:    errors.New("max constraint must be a double"),
			},
			{
				name:             "6. Command is too short",
				presetValue:      nil,
				key:              "ZcountKey6",
				command:          []string{"ZCOUNT"},
				expectedResponse: 0,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "7. Command too long",
				presetValue:      nil,
				key:              "ZcountKey7",
				command:          []string{"ZCOUNT", "ZcountKey4", "min", "max", "count"},
				expectedResponse: 0,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "8. Throw error when value at the key is not a sorted set",
				presetValue:      "Default value",
				key:              "ZcountKey8",
				command:          []string{"ZCOUNT", "ZcountKey8", "1", "10"},
				expectedResponse: 0,
				expectedError:    errors.New("value at ZcountKey8 is not a sorted set"),
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
					case *sorted_set.SortedSet:
						command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(test.key)}
						for _, member := range test.presetValue.(*sorted_set.SortedSet).GetAll() {
							command = append(command, []resp.Value{
								resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
								resp.StringValue(string(member.Value)),
							}...)
						}
						expected = strconv.Itoa(test.presetValue.(*sorted_set.SortedSet).Cardinality())
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

	t.Run("Test_HandleZLEXCOUNT", func(t *testing.T) {
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
				name: "1. Get entire count using infinity boundaries",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "e", Score: sorted_set.Score(1)},
					{Value: "f", Score: sorted_set.Score(1)},
					{Value: "g", Score: sorted_set.Score(1)},
					{Value: "h", Score: sorted_set.Score(1)},
					{Value: "i", Score: sorted_set.Score(1)},
					{Value: "j", Score: sorted_set.Score(1)},
					{Value: "k", Score: sorted_set.Score(1)},
				}),
				key:              "ZlexCountKey1",
				command:          []string{"ZLEXCOUNT", "ZlexCountKey1", "f", "j"},
				expectedResponse: 5,
				expectedError:    nil,
			},
			{
				name: "2. Return 0 when the members do not have the same score",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "a", Score: sorted_set.Score(5.5)},
					{Value: "b", Score: sorted_set.Score(67.77)},
					{Value: "c", Score: sorted_set.Score(10)},
					{Value: "d", Score: sorted_set.Score(1083.13)},
					{Value: "e", Score: sorted_set.Score(11)},
					{Value: "f", Score: sorted_set.Score(math.Inf(-1))},
					{Value: "g", Score: sorted_set.Score(math.Inf(1))},
				}),
				key:              "ZlexCountKey2",
				command:          []string{"ZLEXCOUNT", "ZlexCountKey2", "a", "b"},
				expectedResponse: 0,
				expectedError:    nil,
			},
			{
				name:             "3. Return 0 when the key does not exist",
				presetValue:      nil,
				key:              "ZlexCountKey3",
				command:          []string{"ZLEXCOUNT", "ZlexCountKey3", "a", "z"},
				expectedResponse: 0,
				expectedError:    nil,
			},
			{
				name:             "4. Return error when the value at the key is not a sorted set",
				presetValue:      "Default value",
				key:              "ZlexCountKey4",
				command:          []string{"ZLEXCOUNT", "ZlexCountKey4", "a", "z"},
				expectedResponse: 0,
				expectedError:    errors.New("value at ZlexCountKey4 is not a sorted set"),
			},
			{
				name:             "5. Command is too short",
				presetValue:      nil,
				key:              "ZlexCountKey5",
				command:          []string{"ZLEXCOUNT"},
				expectedResponse: 0,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "6. Command too long",
				presetValue:      nil,
				key:              "ZlexCountKey6",
				command:          []string{"ZLEXCOUNT", "ZlexCountKey6", "min", "max", "count"},
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
					case *sorted_set.SortedSet:
						command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(test.key)}
						for _, member := range test.presetValue.(*sorted_set.SortedSet).GetAll() {
							command = append(command, []resp.Value{
								resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
								resp.StringValue(string(member.Value)),
							}...)
						}
						expected = strconv.Itoa(test.presetValue.(*sorted_set.SortedSet).Cardinality())
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

	t.Run("Test_HandleZDIFF", func(t *testing.T) {
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
			expectedResponse [][]string
			expectedError    error
		}{
			{
				name: "1. Get the difference between 2 sorted sets without scores.",
				presetValues: map[string]interface{}{
					"ZdiffKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1},
						{Value: "two", Score: 2},
						{Value: "three", Score: 3},
						{Value: "four", Score: 4},
					}),
					"ZdiffKey2": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "three", Score: 3},
						{Value: "four", Score: 4},
						{Value: "five", Score: 5},
						{Value: "six", Score: 6},
						{Value: "seven", Score: 7},
						{Value: "eight", Score: 8},
					}),
				},
				command:          []string{"ZDIFF", "ZdiffKey1", "ZdiffKey2"},
				expectedResponse: [][]string{{"one"}, {"two"}},
				expectedError:    nil,
			},
			{
				name: "2. Get the difference between 2 sorted sets with scores.",
				presetValues: map[string]interface{}{
					"ZdiffKey3": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1},
						{Value: "two", Score: 2},
						{Value: "three", Score: 3},
						{Value: "four", Score: 4},
					}),
					"ZdiffKey4": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "three", Score: 3},
						{Value: "four", Score: 4},
						{Value: "five", Score: 5},
						{Value: "six", Score: 6},
						{Value: "seven", Score: 7},
						{Value: "eight", Score: 8},
					}),
				},
				command:          []string{"ZDIFF", "ZdiffKey3", "ZdiffKey4", "WITHSCORES"},
				expectedResponse: [][]string{{"one", "1"}, {"two", "2"}},
				expectedError:    nil,
			},
			{
				name: "3. Get the difference between 3 sets with scores.",
				presetValues: map[string]interface{}{
					"ZdiffKey5": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZdiffKey6": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11},
					}),
					"ZdiffKey7": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				command:          []string{"ZDIFF", "ZdiffKey5", "ZdiffKey6", "ZdiffKey7", "WITHSCORES"},
				expectedResponse: [][]string{{"three", "3"}, {"four", "4"}, {"five", "5"}, {"six", "6"}},
				expectedError:    nil,
			},
			{
				name: "4. Return sorted set if only one key exists and is a sorted set",
				presetValues: map[string]interface{}{
					"ZdiffKey8": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
				},
				command: []string{"ZDIFF", "ZdiffKey8", "ZdiffKey9", "ZdiffKey10", "WITHSCORES"},
				expectedResponse: [][]string{
					{"one", "1"}, {"two", "2"}, {"three", "3"}, {"four", "4"}, {"five", "5"},
					{"six", "6"}, {"seven", "7"}, {"eight", "8"},
				},
				expectedError: nil,
			},
			{
				name: "5. Throw error when one of the keys is not a sorted set.",
				presetValues: map[string]interface{}{
					"ZdiffKey11": "Default value",
					"ZdiffKey12": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11},
					}),
					"ZdiffKey13": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				command:          []string{"ZDIFF", "ZdiffKey11", "ZdiffKey12", "ZdiffKey13"},
				expectedResponse: nil,
				expectedError:    errors.New("value at ZdiffKey11 is not a sorted set"),
			},
			{
				name:             "6. Command too short",
				command:          []string{"ZDIFF"},
				expectedResponse: [][]string{},
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
						case *sorted_set.SortedSet:
							command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(key)}
							for _, member := range value.(*sorted_set.SortedSet).GetAll() {
								command = append(command, []resp.Value{
									resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
									resp.StringValue(string(member.Value)),
								}...)
							}
							expected = strconv.Itoa(value.(*sorted_set.SortedSet).Cardinality())
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
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), res.Error().Error())
					}
					return
				}

				if len(res.Array()) != len(test.expectedResponse) {
					t.Errorf("expected response array of length %d, got %d", len(test.expectedResponse), len(res.Array()))
				}

				for _, item := range res.Array() {
					value := item.Array()[0].String()
					score := func() string {
						if len(item.Array()) == 2 {
							return item.Array()[1].String()
						}
						return ""
					}()
					if !slices.ContainsFunc(test.expectedResponse, func(expected []string) bool {
						return expected[0] == value
					}) {
						t.Errorf("unexpected member \"%s\" in response", value)
					}
					if score != "" {
						for _, expected := range test.expectedResponse {
							if expected[0] == value && expected[1] != score {
								t.Errorf("expected score for member \"%s\" to be %s, got %s", value, expected[1], score)
							}
						}
					}
				}
			})
		}
	})

	t.Run("Test_HandleZDIFFSTORE", func(t *testing.T) {
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
			expectedValue    *sorted_set.SortedSet
			expectedResponse int
			expectedError    error
		}{
			{
				name: "1. Get the difference between 2 sorted sets.",
				presetValues: map[string]interface{}{
					"ZdiffStoreKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5},
					}),
					"ZdiffStoreKey2": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
				},
				destination:      "ZdiffStoreDestinationKey1",
				command:          []string{"ZDIFFSTORE", "ZdiffStoreDestinationKey1", "ZdiffStoreKey1", "ZdiffStoreKey2"},
				expectedValue:    sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}, {Value: "two", Score: 2}}),
				expectedResponse: 2,
				expectedError:    nil,
			},
			{
				name: "2. Get the difference between 3 sorted sets.",
				presetValues: map[string]interface{}{
					"ZdiffStoreKey3": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZdiffStoreKey4": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11},
					}),
					"ZdiffStoreKey5": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				destination: "ZdiffStoreDestinationKey2",
				command:     []string{"ZDIFFSTORE", "ZdiffStoreDestinationKey2", "ZdiffStoreKey3", "ZdiffStoreKey4", "ZdiffStoreKey5"},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "three", Score: 3}, {Value: "four", Score: 4},
					{Value: "five", Score: 5}, {Value: "six", Score: 6},
				}),
				expectedResponse: 4,
				expectedError:    nil,
			},
			{
				name: "3. Return base sorted set element if base set is the only existing key provided and is a valid sorted set",
				presetValues: map[string]interface{}{
					"ZdiffStoreKey6": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
				},
				destination: "ZdiffStoreDestinationKey3",
				command:     []string{"ZDIFFSTORE", "ZdiffStoreDestinationKey3", "ZdiffStoreKey6", "ZdiffStoreKey7", "ZdiffStoreKey8"},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 1}, {Value: "two", Score: 2},
					{Value: "three", Score: 3}, {Value: "four", Score: 4},
					{Value: "five", Score: 5}, {Value: "six", Score: 6},
					{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
				}),
				expectedResponse: 8,
				expectedError:    nil,
			},
			{
				name: "4. Throw error when base sorted set is not a set.",
				presetValues: map[string]interface{}{
					"ZdiffStoreKey9": "Default value",
					"ZdiffStoreKey10": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11},
					}),
					"ZdiffStoreKey11": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				destination:      "ZdiffStoreDestinationKey4",
				command:          []string{"ZDIFFSTORE", "ZdiffStoreDestinationKey4", "ZdiffStoreKey9", "ZdiffStoreKey10", "ZdiffStoreKey11"},
				expectedValue:    nil,
				expectedResponse: 0,
				expectedError:    errors.New("value at ZdiffStoreKey9 is not a sorted set"),
			},
			{
				name:        "5. Return 0 when base set is non-existent.",
				destination: "ZdiffStoreDestinationKey5",
				presetValues: map[string]interface{}{
					"ZdiffStoreKey12": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11},
					}),
					"ZdiffStoreKey13": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				command:          []string{"ZDIFFSTORE", "ZdiffStoreDestinationKey5", "non-existent", "ZdiffStoreKey12", "ZdiffStoreKey13"},
				expectedValue:    nil,
				expectedResponse: 0,
				expectedError:    nil,
			},
			{
				name:             "6. Command too short",
				command:          []string{"ZDIFFSTORE", "ZdiffStoreDestinationKey6"},
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
						case *sorted_set.SortedSet:
							command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(key)}
							for _, member := range value.(*sorted_set.SortedSet).GetAll() {
								command = append(command, []resp.Value{
									resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
									resp.StringValue(string(member.Value)),
								}...)
							}
							expected = strconv.Itoa(value.(*sorted_set.SortedSet).Cardinality())
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
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), res.Error().Error())
					}
					return
				}

				if res.Integer() != test.expectedResponse {
					t.Errorf("expected response %d, got %d", test.expectedResponse, res.Integer())
				}

				// Check if the resulting sorted set has the expected members/scores
				if test.expectedValue == nil {
					return
				}

				if err = client.WriteArray([]resp.Value{
					resp.StringValue("ZRANGE"),
					resp.StringValue(test.destination),
					resp.StringValue("-inf"),
					resp.StringValue("+inf"),
					resp.StringValue("BYSCORE"),
					resp.StringValue("WITHSCORES"),
				}); err != nil {
					t.Error(err)
				}

				res, _, err = client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if len(res.Array()) != test.expectedValue.Cardinality() {
					t.Errorf("expected resulting set %s to have cardinality %d, got %d",
						test.destination, test.expectedValue.Cardinality(), len(res.Array()))
				}

				for _, member := range res.Array() {
					value := sorted_set.Value(member.Array()[0].String())
					score := sorted_set.Score(member.Array()[1].Float())
					if !test.expectedValue.Contains(value) {
						t.Errorf("unexpected value %s in resulting sorted set", value)
					}
					if test.expectedValue.Get(value).Score != score {
						t.Errorf("expected value %s to have score %v, got %v", value, test.expectedValue.Get(value).Score, score)
					}
				}
			})
		}
	})

	t.Run("Test_HandleZINCRBY", func(t *testing.T) {
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
			expectedValue    *sorted_set.SortedSet
			expectedResponse string
			expectedError    error
		}{
			{
				name: "1. Successfully increment by int. Return the new score",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 1}, {Value: "two", Score: 2},
					{Value: "three", Score: 3}, {Value: "four", Score: 4},
					{Value: "five", Score: 5},
				}),
				key:     "ZincrbyKey1",
				command: []string{"ZINCRBY", "ZincrbyKey1", "5", "one"},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 6}, {Value: "two", Score: 2},
					{Value: "three", Score: 3}, {Value: "four", Score: 4},
					{Value: "five", Score: 5},
				}),
				expectedResponse: "6",
				expectedError:    nil,
			},
			{
				name: "2. Successfully increment by float. Return new score",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 1}, {Value: "two", Score: 2},
					{Value: "three", Score: 3}, {Value: "four", Score: 4},
					{Value: "five", Score: 5},
				}),
				key:     "ZincrbyKey2",
				command: []string{"ZINCRBY", "ZincrbyKey2", "346.785", "one"},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 347.785}, {Value: "two", Score: 2},
					{Value: "three", Score: 3}, {Value: "four", Score: 4},
					{Value: "five", Score: 5},
				}),
				expectedResponse: "347.785",
				expectedError:    nil,
			},
			{
				name:        "3. Increment on non-existent sorted set will create the set with the member and increment as its score",
				presetValue: nil,
				key:         "ZincrbyKey3",
				command:     []string{"ZINCRBY", "ZincrbyKey3", "346.785", "one"},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 346.785},
				}),
				expectedResponse: "346.785",
				expectedError:    nil,
			},
			{
				name: "4. Increment score to +inf",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 1}, {Value: "two", Score: 2},
					{Value: "three", Score: 3}, {Value: "four", Score: 4},
					{Value: "five", Score: 5},
				}),
				key:     "ZincrbyKey4",
				command: []string{"ZINCRBY", "ZincrbyKey4", "+inf", "one"},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: sorted_set.Score(math.Inf(1))}, {Value: "two", Score: 2},
					{Value: "three", Score: 3}, {Value: "four", Score: 4},
					{Value: "five", Score: 5},
				}),
				expectedResponse: "+Inf",
				expectedError:    nil,
			},
			{
				name: "5. Increment score to -inf",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 1}, {Value: "two", Score: 2},
					{Value: "three", Score: 3}, {Value: "four", Score: 4},
					{Value: "five", Score: 5},
				}),
				key:     "ZincrbyKey5",
				command: []string{"ZINCRBY", "ZincrbyKey5", "-inf", "one"},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: sorted_set.Score(math.Inf(-1))}, {Value: "two", Score: 2},
					{Value: "three", Score: 3}, {Value: "four", Score: 4},
					{Value: "five", Score: 5},
				}),
				expectedResponse: "-Inf",
				expectedError:    nil,
			},
			{
				name: "6. Incrementing score by negative increment should lower the score",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 1}, {Value: "two", Score: 2},
					{Value: "three", Score: 3}, {Value: "four", Score: 4},
					{Value: "five", Score: 5},
				}),
				key:     "ZincrbyKey6",
				command: []string{"ZINCRBY", "ZincrbyKey6", "-2.5", "five"},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 1}, {Value: "two", Score: 2},
					{Value: "three", Score: 3}, {Value: "four", Score: 4},
					{Value: "five", Score: 2.5},
				}),
				expectedResponse: "2.5",
				expectedError:    nil,
			},
			{
				name:             "7. Return error when attempting to increment on a value that is not a valid sorted set",
				presetValue:      "Default value",
				key:              "ZincrbyKey7",
				command:          []string{"ZINCRBY", "ZincrbyKey7", "-2.5", "five"},
				expectedValue:    nil,
				expectedResponse: "",
				expectedError:    errors.New("value at ZincrbyKey7 is not a sorted set"),
			},
			{
				name: "8. Return error when trying to increment a member that already has score -inf",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: sorted_set.Score(math.Inf(-1))},
				}),
				key:     "ZincrbyKey8",
				command: []string{"ZINCRBY", "ZincrbyKey8", "2.5", "one"},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: sorted_set.Score(math.Inf(-1))},
				}),
				expectedResponse: "",
				expectedError:    errors.New("cannot increment -inf or +inf"),
			},
			{
				name: "9. Return error when trying to increment a member that already has score +inf",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: sorted_set.Score(math.Inf(1))},
				}),
				key:     "ZincrbyKey9",
				command: []string{"ZINCRBY", "ZincrbyKey9", "2.5", "one"},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: sorted_set.Score(math.Inf(-1))},
				}),
				expectedResponse: "",
				expectedError:    errors.New("cannot increment -inf or +inf"),
			},
			{
				name: "10. Return error when increment is not a valid number",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 1},
				}),
				key:     "ZincrbyKey10",
				command: []string{"ZINCRBY", "ZincrbyKey10", "increment", "one"},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 1},
				}),
				expectedResponse: "",
				expectedError:    errors.New("increment must be a double"),
			},
			{
				name:             "11. Command too short",
				key:              "ZincrbyKey11",
				command:          []string{"ZINCRBY", "ZincrbyKey11", "one"},
				expectedResponse: "",
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "12. Command too long",
				key:              "ZincrbyKey12",
				command:          []string{"ZINCRBY", "ZincrbyKey12", "one", "1", "2"},
				expectedResponse: "",
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
					case *sorted_set.SortedSet:
						command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(test.key)}
						for _, member := range test.presetValue.(*sorted_set.SortedSet).GetAll() {
							command = append(command, []resp.Value{
								resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
								resp.StringValue(string(member.Value)),
							}...)
						}
						expected = strconv.Itoa(test.presetValue.(*sorted_set.SortedSet).Cardinality())
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

				// Check if the resulting sorted set has the expected members/scores
				if test.expectedValue == nil {
					return
				}

				if err = client.WriteArray([]resp.Value{
					resp.StringValue("ZRANGE"),
					resp.StringValue(test.key),
					resp.StringValue("-inf"),
					resp.StringValue("+inf"),
					resp.StringValue("BYSCORE"),
					resp.StringValue("WITHSCORES"),
				}); err != nil {
					t.Error(err)
				}

				res, _, err = client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if len(res.Array()) != test.expectedValue.Cardinality() {
					t.Errorf("expected resulting set %s to have cardinality %d, got %d",
						test.key, test.expectedValue.Cardinality(), len(res.Array()))
				}

				for _, member := range res.Array() {
					value := sorted_set.Value(member.Array()[0].String())
					score := sorted_set.Score(member.Array()[1].Float())
					if !test.expectedValue.Contains(value) {
						t.Errorf("unexpected value %s in resulting sorted set", value)
					}
					if test.expectedValue.Get(value).Score != score {
						t.Errorf("expected value %s to have score %v, got %v", value, test.expectedValue.Get(value).Score, score)
					}
				}
			})
		}
	})

	t.Run("Test_HandleZMPOP", func(t *testing.T) {
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
			presetValues     map[string]interface{}
			command          []string
			expectedValues   map[string]*sorted_set.SortedSet
			expectedResponse [][]string
			expectedError    error
		}{
			{
				name:   "1. Successfully pop one min element by default",
				preset: true,
				presetValues: map[string]interface{}{
					"ZmpopKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5},
					}),
				},
				command: []string{"ZMPOP", "ZmpopKey1"},
				expectedValues: map[string]*sorted_set.SortedSet{
					"ZmpopKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5},
					}),
				},
				expectedResponse: [][]string{
					{"one", "1"},
				},
				expectedError: nil,
			},
			{
				name:   "2. Successfully pop one min element by specifying MIN",
				preset: true,
				presetValues: map[string]interface{}{
					"ZmpopKey2": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5},
					}),
				},
				command: []string{"ZMPOP", "ZmpopKey2", "MIN"},
				expectedValues: map[string]*sorted_set.SortedSet{
					"ZmpopKey2": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5},
					}),
				},
				expectedResponse: [][]string{
					{"one", "1"},
				},
				expectedError: nil,
			},
			{
				name:   "3. Successfully pop one max element by specifying MAX modifier",
				preset: true,
				presetValues: map[string]interface{}{
					"ZmpopKey3": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5},
					}),
				},
				command: []string{"ZMPOP", "ZmpopKey3", "MAX"},
				expectedValues: map[string]*sorted_set.SortedSet{
					"ZmpopKey3": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
					}),
				},
				expectedResponse: [][]string{
					{"five", "5"},
				},
				expectedError: nil,
			},
			{
				name:   "4. Successfully pop multiple min elements",
				preset: true,
				presetValues: map[string]interface{}{
					"ZmpopKey4": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
					}),
				},
				command: []string{"ZMPOP", "ZmpopKey4", "MIN", "COUNT", "5"},
				expectedValues: map[string]*sorted_set.SortedSet{
					"ZmpopKey4": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "six", Score: 6},
					}),
				},
				expectedResponse: [][]string{
					{"one", "1"}, {"two", "2"}, {"three", "3"},
					{"four", "4"}, {"five", "5"},
				},
				expectedError: nil,
			},
			{
				name:   "5. Successfully pop multiple max elements",
				preset: true,
				presetValues: map[string]interface{}{
					"ZmpopKey5": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
					}),
				},
				command: []string{"ZMPOP", "ZmpopKey5", "MAX", "COUNT", "5"},
				expectedValues: map[string]*sorted_set.SortedSet{
					"ZmpopKey5": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1},
					}),
				},
				expectedResponse: [][]string{{"two", "2"}, {"three", "3"}, {"four", "4"}, {"five", "5"}, {"six", "6"}},
				expectedError:    nil,
			},
			{
				name:   "6. Successfully pop elements from the first set which is non-empty",
				preset: true,
				presetValues: map[string]interface{}{
					"ZmpopKey7": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
					}),
				},
				command: []string{"ZMPOP", "ZmpopKey6", "ZmpopKey7", "MAX", "COUNT", "5"},
				expectedValues: map[string]*sorted_set.SortedSet{
					"ZmpopKey6": sorted_set.NewSortedSet([]sorted_set.MemberParam{}),
					"ZmpopKey7": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1},
					}),
				},
				expectedResponse: [][]string{{"two", "2"}, {"three", "3"}, {"four", "4"}, {"five", "5"}, {"six", "6"}},
				expectedError:    nil,
			},
			{
				name:   "7. Skip the non-set items and pop elements from the first non-empty sorted set found",
				preset: true,
				presetValues: map[string]interface{}{
					"ZmpopKey8": "Default value",
					"ZmpopKey9": "56",
					"ZmpopKey11": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
					}),
				},
				command: []string{"ZMPOP", "ZmpopKey8", "ZmpopKey9", "ZmpopKey10", "ZmpopKey11", "MIN", "COUNT", "5"},
				expectedValues: map[string]*sorted_set.SortedSet{
					"ZmpopKey10": sorted_set.NewSortedSet([]sorted_set.MemberParam{}),
					"ZmpopKey11": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "six", Score: 6},
					}),
				},
				expectedResponse: [][]string{{"one", "1"}, {"two", "2"}, {"three", "3"}, {"four", "4"}, {"five", "5"}},
				expectedError:    nil,
			},
			{
				name:          "9. Return error when count is a negative integer",
				preset:        false,
				command:       []string{"ZMPOP", "ZmpopKey8", "MAX", "COUNT", "-20"},
				expectedError: errors.New("count must be a positive integer"),
			},
			{
				name:          "9. Command too short",
				preset:        false,
				command:       []string{"ZMPOP"},
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
						case *sorted_set.SortedSet:
							command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(key)}
							for _, member := range value.(*sorted_set.SortedSet).GetAll() {
								command = append(command, []resp.Value{
									resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
									resp.StringValue(string(member.Value)),
								}...)
							}
							expected = strconv.Itoa(value.(*sorted_set.SortedSet).Cardinality())
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
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), res.Error().Error())
					}
					return
				}

				if len(res.Array()) != len(test.expectedResponse) {
					t.Errorf("expected response array of length %d, got %d", len(test.expectedResponse), len(res.Array()))
				}

				for _, item := range res.Array() {
					value := item.Array()[0].String()
					score := func() string {
						if len(item.Array()) == 2 {
							return item.Array()[1].String()
						}
						return ""
					}()
					if !slices.ContainsFunc(test.expectedResponse, func(expected []string) bool {
						return expected[0] == value
					}) {
						t.Errorf("unexpected member \"%s\" in response", value)
					}
					if score != "" {
						for _, expected := range test.expectedResponse {
							if expected[0] == value && expected[1] != score {
								t.Errorf("expected score for member \"%s\" to be %s, got %s", value, expected[1], score)
							}
						}
					}
				}

				// Check if the resulting sorted set has the expected members/scores
				for key, expectedSortedSet := range test.expectedValues {
					if expectedSortedSet == nil {
						continue
					}

					if err = client.WriteArray([]resp.Value{
						resp.StringValue("ZRANGE"),
						resp.StringValue(key),
						resp.StringValue("-inf"),
						resp.StringValue("+inf"),
						resp.StringValue("BYSCORE"),
						resp.StringValue("WITHSCORES"),
					}); err != nil {
						t.Error(err)
					}

					res, _, err = client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if len(res.Array()) != expectedSortedSet.Cardinality() {
						t.Errorf("expected resulting set %s to have cardinality %d, got %d",
							key, expectedSortedSet.Cardinality(), len(res.Array()))
					}

					for _, member := range res.Array() {
						value := sorted_set.Value(member.Array()[0].String())
						score := sorted_set.Score(member.Array()[1].Float())
						if !expectedSortedSet.Contains(value) {
							t.Errorf("unexpected value %s in resulting sorted set", value)
						}
						if expectedSortedSet.Get(value).Score != score {
							t.Errorf("expected value %s to have score %v, got %v",
								value, expectedSortedSet.Get(value).Score, score)
						}
					}
				}
			})
		}
	})

	t.Run("Test_HandleZPOP", func(t *testing.T) {
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
			presetValues     map[string]interface{}
			command          []string
			expectedValues   map[string]*sorted_set.SortedSet
			expectedResponse [][]string
			expectedError    error
		}{
			{
				name:   "1. Successfully pop one min element by default",
				preset: true,
				presetValues: map[string]interface{}{
					"ZmpopMinKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5},
					}),
				},
				command: []string{"ZPOPMIN", "ZmpopMinKey1"},
				expectedValues: map[string]*sorted_set.SortedSet{
					"ZmpopMinKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5},
					}),
				},
				expectedResponse: [][]string{
					{"one", "1"},
				},
				expectedError: nil,
			},
			{
				name:   "2. Successfully pop one max element by default",
				preset: true,
				presetValues: map[string]interface{}{
					"ZmpopMaxKey2": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5},
					}),
				},
				command: []string{"ZPOPMAX", "ZmpopMaxKey2"},
				expectedValues: map[string]*sorted_set.SortedSet{
					"ZmpopMaxKey2": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
					}),
				},
				expectedResponse: [][]string{
					{"five", "5"},
				},
				expectedError: nil,
			},
			{
				name:   "3. Successfully pop multiple min elements",
				preset: true,
				presetValues: map[string]interface{}{
					"ZmpopMinKey3": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
					}),
				},
				command: []string{"ZPOPMIN", "ZmpopMinKey3", "5"},
				expectedValues: map[string]*sorted_set.SortedSet{
					"ZmpopMinKey3": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "six", Score: 6},
					}),
				},
				expectedResponse: [][]string{
					{"one", "1"}, {"two", "2"}, {"three", "3"},
					{"four", "4"}, {"five", "5"},
				},
				expectedError: nil,
			},
			{
				name:   "4. Successfully pop multiple max elements",
				preset: true,
				presetValues: map[string]interface{}{
					"ZmpopMaxKey4": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
					}),
				},
				command: []string{"ZPOPMAX", "ZmpopMaxKey4", "5"},
				expectedValues: map[string]*sorted_set.SortedSet{
					"ZmpopMaxKey4": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1},
					}),
				},
				expectedResponse: [][]string{{"two", "2"}, {"three", "3"}, {"four", "4"}, {"five", "5"}, {"six", "6"}},
				expectedError:    nil,
			},
			{
				name:   "5. Throw an error when trying to pop from an element that's not a sorted set",
				preset: true,
				presetValues: map[string]interface{}{
					"ZmpopMinKey5": "Default value",
				},
				command:          []string{"ZPOPMIN", "ZmpopMinKey5"},
				expectedValues:   nil,
				expectedResponse: nil,
				expectedError:    errors.New("value at key ZmpopMinKey5 is not a sorted set"),
			},
			{
				name:          "6. Command too short",
				preset:        false,
				command:       []string{"ZPOPMAX"},
				expectedError: errors.New(constants.WrongArgsResponse),
			},
			{
				name:          "7. Command too long",
				preset:        false,
				command:       []string{"ZPOPMAX", "ZmpopMaxKey7", "6", "3"},
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
						case *sorted_set.SortedSet:
							command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(key)}
							for _, member := range value.(*sorted_set.SortedSet).GetAll() {
								command = append(command, []resp.Value{
									resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
									resp.StringValue(string(member.Value)),
								}...)
							}
							expected = strconv.Itoa(value.(*sorted_set.SortedSet).Cardinality())
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
					t.Errorf("expected response array of length %d, got %d", len(test.expectedResponse), len(res.Array()))
				}

				for _, item := range res.Array() {
					value := item.Array()[0].String()
					score := func() string {
						if len(item.Array()) == 2 {
							return item.Array()[1].String()
						}
						return ""
					}()
					if !slices.ContainsFunc(test.expectedResponse, func(expected []string) bool {
						return expected[0] == value
					}) {
						t.Errorf("unexpected member \"%s\" in response", value)
					}
					if score != "" {
						for _, expected := range test.expectedResponse {
							if expected[0] == value && expected[1] != score {
								t.Errorf("expected score for member \"%s\" to be %s, got %s", value, expected[1], score)
							}
						}
					}
				}

				// Check if the resulting sorted set has the expected members/scores
				for key, expectedSortedSet := range test.expectedValues {
					if expectedSortedSet == nil {
						continue
					}

					if err = client.WriteArray([]resp.Value{
						resp.StringValue("ZRANGE"),
						resp.StringValue(key),
						resp.StringValue("-inf"),
						resp.StringValue("+inf"),
						resp.StringValue("BYSCORE"),
						resp.StringValue("WITHSCORES"),
					}); err != nil {
						t.Error(err)
					}

					res, _, err = client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if len(res.Array()) != expectedSortedSet.Cardinality() {
						t.Errorf("expected resulting set %s to have cardinality %d, got %d",
							key, expectedSortedSet.Cardinality(), len(res.Array()))
					}

					for _, member := range res.Array() {
						value := sorted_set.Value(member.Array()[0].String())
						score := sorted_set.Score(member.Array()[1].Float())
						if !expectedSortedSet.Contains(value) {
							t.Errorf("unexpected value %s in resulting sorted set", value)
						}
						if expectedSortedSet.Get(value).Score != score {
							t.Errorf("expected value %s to have score %v, got %v",
								value, expectedSortedSet.Get(value).Score, score)
						}
					}
				}
			})
		}
	})

	t.Run("Test_HandleZMSCORE", func(t *testing.T) {
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
				// 1. Return multiple scores from the sorted set.
				// Return nil for elements that do not exist in the sorted set.
				name: "1. Return multiple scores from the sorted set.",
				presetValues: map[string]interface{}{
					"ZmScoreKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1.1}, {Value: "two", Score: 245},
						{Value: "three", Score: 3}, {Value: "four", Score: 4.055},
						{Value: "five", Score: 5},
					}),
				},
				command:          []string{"ZMSCORE", "ZmScoreKey1", "one", "none", "two", "one", "three", "four", "none", "five"},
				expectedResponse: []string{"1.1", "", "245", "1.1", "3", "4.055", "", "5"},
				expectedError:    nil,
			},
			{
				name:             "2. If key does not exist, return empty array",
				presetValues:     nil,
				command:          []string{"ZMSCORE", "ZmScoreKey2", "one", "two", "three", "four"},
				expectedResponse: []string{},
				expectedError:    nil,
			},
			{
				name:          "3. Throw error when trying to find scores from elements that are not sorted sets",
				presetValues:  map[string]interface{}{"ZmScoreKey3": "Default value"},
				command:       []string{"ZMSCORE", "ZmScoreKey3", "one", "two", "three"},
				expectedError: errors.New("value at ZmScoreKey3 is not a sorted set"),
			},
			{
				name:          "9. Command too short",
				command:       []string{"ZMSCORE"},
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
						case *sorted_set.SortedSet:
							command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(key)}
							for _, member := range value.(*sorted_set.SortedSet).GetAll() {
								command = append(command, []resp.Value{
									resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
									resp.StringValue(string(member.Value)),
								}...)
							}
							expected = strconv.Itoa(value.(*sorted_set.SortedSet).Cardinality())
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
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), res.Error().Error())
					}
					return
				}

				if len(res.Array()) != len(test.expectedResponse) {
					t.Errorf("expected response array of length %d, got %d", len(test.expectedResponse), len(res.Array()))
				}

				for i := 0; i < len(res.Array()); i++ {
					if test.expectedResponse[i] != res.Array()[i].String() {
						t.Errorf("expected element at index %d to be \"%s\", got %s",
							i, test.expectedResponse[i], res.Array()[i].String())
					}
				}
			})
		}
	})

	t.Run("Test_HandleZSCORE", func(t *testing.T) {
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
			expectedResponse string
			expectedError    error
		}{
			{
				name: "1. Return score from a sorted set.",
				presetValues: map[string]interface{}{
					"ZscoreKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1.1}, {Value: "two", Score: 245},
						{Value: "three", Score: 3}, {Value: "four", Score: 4.055},
						{Value: "five", Score: 5},
					}),
				},
				command:          []string{"ZSCORE", "ZscoreKey1", "four"},
				expectedResponse: "4.055",
				expectedError:    nil,
			},
			{
				name:             "2. If key does not exist, return nil value",
				presetValues:     nil,
				command:          []string{"ZSCORE", "ZscoreKey2", "one"},
				expectedResponse: "",
				expectedError:    nil,
			},
			{
				name: "3. If key exists and is a sorted set, but the member does not exist, return nil",
				presetValues: map[string]interface{}{
					"ZscoreKey3": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1.1}, {Value: "two", Score: 245},
						{Value: "three", Score: 3}, {Value: "four", Score: 4.055},
						{Value: "five", Score: 5},
					}),
				},
				command:          []string{"ZSCORE", "ZscoreKey3", "non-existent"},
				expectedResponse: "",
				expectedError:    nil,
			},
			{
				name:          "4. Throw error when trying to find scores from elements that are not sorted sets",
				presetValues:  map[string]interface{}{"ZscoreKey4": "Default value"},
				command:       []string{"ZSCORE", "ZscoreKey4", "one"},
				expectedError: errors.New("value at ZscoreKey4 is not a sorted set"),
			},
			{
				name:          "5. Command too short",
				command:       []string{"ZSCORE"},
				expectedError: errors.New(constants.WrongArgsResponse),
			},
			{
				name:          "6. Command too long",
				command:       []string{"ZSCORE", "ZscoreKey5", "one", "two"},
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
						case *sorted_set.SortedSet:
							command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(key)}
							for _, member := range value.(*sorted_set.SortedSet).GetAll() {
								command = append(command, []resp.Value{
									resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
									resp.StringValue(string(member.Value)),
								}...)
							}
							expected = strconv.Itoa(value.(*sorted_set.SortedSet).Cardinality())
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
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), res.Error().Error())
					}
					return
				}

				if res.String() != test.expectedResponse {
					t.Errorf("expected response \"%s\", got \"%s\"", test.expectedResponse, res.String())
				}
			})
		}
	})

	t.Run("Test_HandleZRANDMEMBER", func(t *testing.T) {
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
			expectedResponse [][]string
			expectedError    error
		}{
			{
				// 1. Return multiple random elements without removing them.
				// Count is positive, do not allow repeated elements
				name: "1. Return multiple random elements without removing them.",
				key:  "ZrandMemberKey1",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 1}, {Value: "two", Score: 2}, {Value: "three", Score: 3}, {Value: "four", Score: 4},
					{Value: "five", Score: 5}, {Value: "six", Score: 6}, {Value: "seven", Score: 7}, {Value: "eight", Score: 8},
				}),
				command:       []string{"ZRANDMEMBER", "ZrandMemberKey1", "3"},
				expectedValue: 8,
				allowRepeat:   false,
				expectedResponse: [][]string{
					{"one"}, {"two"}, {"three"}, {"four"},
					{"five"}, {"six"}, {"seven"}, {"eight"},
				},
				expectedError: nil,
			},
			{
				// 2. Return multiple random elements and their scores without removing them.
				// Count is negative, so allow repeated numbers.
				name: "2. Return multiple random elements and their scores without removing them.",
				key:  "ZrandMemberKey2",
				presetValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 1}, {Value: "two", Score: 2}, {Value: "three", Score: 3}, {Value: "four", Score: 4},
					{Value: "five", Score: 5}, {Value: "six", Score: 6}, {Value: "seven", Score: 7}, {Value: "eight", Score: 8},
				}),
				command:       []string{"ZRANDMEMBER", "ZrandMemberKey2", "-5", "WITHSCORES"},
				expectedValue: 8,
				allowRepeat:   true,
				expectedResponse: [][]string{
					{"one", "1"}, {"two", "2"}, {"three", "3"}, {"four", "4"},
					{"five", "5"}, {"six", "6"}, {"seven", "7"}, {"eight", "8"},
				},
				expectedError: nil,
			},
			{
				name:          "3. Return error when the source key is not a sorted set.",
				key:           "ZrandMemberKey3",
				presetValue:   "Default value",
				command:       []string{"ZRANDMEMBER", "ZrandMemberKey3"},
				expectedValue: 0,
				expectedError: errors.New("value at ZrandMemberKey3 is not a sorted set"),
			},
			{
				name:          "5. Command too short",
				command:       []string{"ZRANDMEMBER"},
				expectedError: errors.New(constants.WrongArgsResponse),
			},
			{
				name:          "6. Command too long",
				command:       []string{"ZRANDMEMBER", "source5", "source6", "member1", "member2"},
				expectedError: errors.New(constants.WrongArgsResponse),
			},
			{
				name:          "7. Throw error when count is not an integer",
				command:       []string{"ZRANDMEMBER", "ZrandMemberKey1", "count"},
				expectedError: errors.New("count must be an integer"),
			},
			{
				name:          "8. Throw error when the fourth argument is not WITHSCORES",
				command:       []string{"ZRANDMEMBER", "ZrandMemberKey1", "8", "ANOTHER"},
				expectedError: errors.New("last option must be WITHSCORES"),
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
					case *sorted_set.SortedSet:
						command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(test.key)}
						for _, member := range test.presetValue.(*sorted_set.SortedSet).GetAll() {
							command = append(command, []resp.Value{
								resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
								resp.StringValue(string(member.Value)),
							}...)
						}
						expected = strconv.Itoa(test.presetValue.(*sorted_set.SortedSet).Cardinality())
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

				// Check that each of the returned elements is in the expected response.
				for _, item := range res.Array() {
					value := sorted_set.Value(item.Array()[0].String())
					if !slices.ContainsFunc(test.expectedResponse, func(expected []string) bool {
						return expected[0] == string(value)
					}) {
						t.Errorf("unexected element \"%s\" in response", value)
					}
					for _, expected := range test.expectedResponse {
						if len(item.Array()) != len(expected) {
							t.Errorf("expected response for element \"%s\" to have length %d, got %d",
								value, len(expected), len(item.Array()))
						}
						if expected[0] != string(value) {
							continue
						}
						if len(expected) == 2 {
							score := item.Array()[1].String()
							if expected[1] != score {
								t.Errorf("expected score for memebr \"%s\" to be %s, got %s", value, expected[1], score)
							}
						}
					}
				}

				// Check that allowRepeat determines whether elements are repeated or not.
				if !test.allowRepeat {
					ss := sorted_set.NewSortedSet([]sorted_set.MemberParam{})
					for _, item := range res.Array() {
						member := sorted_set.Value(item.Array()[0].String())
						score := func() sorted_set.Score {
							if len(item.Array()) == 2 {
								return sorted_set.Score(item.Array()[1].Float())
							}
							return sorted_set.Score(0)
						}()
						_, err = ss.AddOrUpdate(
							[]sorted_set.MemberParam{{member, score}},
							nil, nil, nil, nil)
						if err != nil {
							t.Error(err)
						}
					}
					if len(res.Array()) != ss.Cardinality() {
						t.Error("unexpected repeated elements in response")
					}
				}
			})
		}
	})

	t.Run("Test_HandleZRANK", func(t *testing.T) {
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
				name: "1. Return element's rank from a sorted set.",
				presetValues: map[string]interface{}{
					"ZrankKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5},
					}),
				},
				command:          []string{"ZRANK", "ZrankKey1", "four"},
				expectedResponse: []string{"3"},
				expectedError:    nil,
			},
			{
				name: "2. Return element's rank from a sorted set with its score.",
				presetValues: map[string]interface{}{
					"ZrankKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 100.1}, {Value: "two", Score: 245},
						{Value: "three", Score: 305.43}, {Value: "four", Score: 411.055},
						{Value: "five", Score: 500},
					}),
				},
				command:          []string{"ZRANK", "ZrankKey1", "four", "WITHSCORES"},
				expectedResponse: []string{"3", "411.055"},
				expectedError:    nil,
			},
			{
				name:             "3. If key does not exist, return nil value",
				presetValues:     nil,
				command:          []string{"ZRANK", "ZrankKey3", "one"},
				expectedResponse: nil,
				expectedError:    nil,
			},
			{
				name: "4. If key exists and is a sorted set, but the member does not exist, return nil",
				presetValues: map[string]interface{}{
					"ZrankKey4": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1.1}, {Value: "two", Score: 245},
						{Value: "three", Score: 3}, {Value: "four", Score: 4.055},
						{Value: "five", Score: 5},
					}),
				},
				command:          []string{"ZRANK", "ZrankKey4", "non-existent"},
				expectedResponse: nil,
				expectedError:    nil,
			},
			{
				name:          "5. Throw error when trying to find scores from elements that are not sorted sets",
				presetValues:  map[string]interface{}{"ZrankKey5": "Default value"},
				command:       []string{"ZRANK", "ZrankKey5", "one"},
				expectedError: errors.New("value at ZrankKey5 is not a sorted set"),
			},
			{
				name:          "5. Command too short",
				command:       []string{"ZRANK"},
				expectedError: errors.New(constants.WrongArgsResponse),
			},
			{
				name:          "6. Command too long",
				command:       []string{"ZRANK", "ZrankKey5", "one", "WITHSCORES", "two"},
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
						case *sorted_set.SortedSet:
							command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(key)}
							for _, member := range value.(*sorted_set.SortedSet).GetAll() {
								command = append(command, []resp.Value{
									resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
									resp.StringValue(string(member.Value)),
								}...)
							}
							expected = strconv.Itoa(value.(*sorted_set.SortedSet).Cardinality())
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
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), res.Error().Error())
					}
					return
				}

				if len(res.Array()) != len(test.expectedResponse) {
					t.Errorf("expected response array of length %d, got %d", len(test.expectedResponse), len(res.Array()))
				}

				for i := 0; i < len(res.Array()); i++ {
					if test.expectedResponse[i] != res.Array()[i].String() {
						t.Errorf("expected element at index %d to be \"%s\", got %s",
							i, test.expectedResponse[i], res.Array()[i].String())
					}
				}
			})
		}
	})

	t.Run("Test_HandleZREM", func(t *testing.T) {
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
			expectedValues   map[string]*sorted_set.SortedSet
			expectedResponse int
			expectedError    error
		}{
			{
				// Successfully remove multiple elements from sorted set, skipping non-existent members.
				// Return deleted count.
				name: "1. Successfully remove multiple elements from sorted set, skipping non-existent members.",
				presetValues: map[string]interface{}{
					"ZremKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
					}),
				},
				command: []string{"ZREM", "ZremKey1", "three", "four", "five", "none", "six", "none", "seven"},
				expectedValues: map[string]*sorted_set.SortedSet{
					"ZremKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2}, {Value: "eight", Score: 8},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
					}),
				},
				expectedResponse: 5,
				expectedError:    nil,
			},
			{
				name:             "2. If key does not exist, return 0",
				presetValues:     nil,
				command:          []string{"ZREM", "ZremKey2", "member"},
				expectedValues:   nil,
				expectedResponse: 0,
				expectedError:    nil,
			},
			{
				name: "3. Return error key is not a sorted set",
				presetValues: map[string]interface{}{
					"ZremKey3": "Default value",
				},
				command:       []string{"ZREM", "ZremKey3", "member"},
				expectedError: errors.New("value at ZremKey3 is not a sorted set"),
			},
			{
				name:          "9. Command too short",
				command:       []string{"ZREM"},
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
						case *sorted_set.SortedSet:
							command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(key)}
							for _, member := range value.(*sorted_set.SortedSet).GetAll() {
								command = append(command, []resp.Value{
									resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
									resp.StringValue(string(member.Value)),
								}...)
							}
							expected = strconv.Itoa(value.(*sorted_set.SortedSet).Cardinality())
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
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), res.Error().Error())
					}
					return
				}

				if res.Integer() != test.expectedResponse {
					t.Errorf("expected response array of length %d, got %d", test.expectedResponse, res.Integer())
				}

				// Check if the resulting sorted set has the expected members/scores
				for key, expectedSortedSet := range test.expectedValues {
					if expectedSortedSet == nil {
						continue
					}

					if err = client.WriteArray([]resp.Value{
						resp.StringValue("ZRANGE"),
						resp.StringValue(key),
						resp.StringValue("-inf"),
						resp.StringValue("+inf"),
						resp.StringValue("BYSCORE"),
						resp.StringValue("WITHSCORES"),
					}); err != nil {
						t.Error(err)
					}

					res, _, err = client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if len(res.Array()) != expectedSortedSet.Cardinality() {
						t.Errorf("expected resulting set %s to have cardinality %d, got %d",
							key, expectedSortedSet.Cardinality(), len(res.Array()))
					}

					for _, member := range res.Array() {
						value := sorted_set.Value(member.Array()[0].String())
						score := sorted_set.Score(member.Array()[1].Float())
						if !expectedSortedSet.Contains(value) {
							t.Errorf("unexpected value %s in resulting sorted set", value)
						}
						if expectedSortedSet.Get(value).Score != score {
							t.Errorf("expected value %s to have score %v, got %v",
								value, expectedSortedSet.Get(value).Score, score)
						}
					}
				}
			})
		}
	})

	t.Run("Test_HandleZREMRANGEBYSCORE", func(t *testing.T) {
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
			expectedValues   map[string]*sorted_set.SortedSet
			expectedResponse int
			expectedError    error
		}{
			{
				name: "1. Successfully remove multiple elements with scores inside the provided range",
				presetValues: map[string]interface{}{
					"ZremRangeByScoreKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
					}),
				},
				command: []string{"ZREMRANGEBYSCORE", "ZremRangeByScoreKey1", "3", "7"},
				expectedValues: map[string]*sorted_set.SortedSet{
					"ZremRangeByScoreKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2}, {Value: "eight", Score: 8},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
					}),
				},
				expectedResponse: 5,
				expectedError:    nil,
			},
			{
				name:             "2. If key does not exist, return 0",
				presetValues:     nil,
				command:          []string{"ZREMRANGEBYSCORE", "ZremRangeByScoreKey2", "2", "4"},
				expectedValues:   nil,
				expectedResponse: 0,
				expectedError:    nil,
			},
			{
				name: "3. Return error key is not a sorted set",
				presetValues: map[string]interface{}{
					"ZremRangeByScoreKey3": "Default value",
				},
				command:       []string{"ZREMRANGEBYSCORE", "ZremRangeByScoreKey3", "4", "4"},
				expectedError: errors.New("value at ZremRangeByScoreKey3 is not a sorted set"),
			},
			{
				name:          "4. Command too short",
				command:       []string{"ZREMRANGEBYSCORE", "ZremRangeByScoreKey4", "3"},
				expectedError: errors.New(constants.WrongArgsResponse),
			},
			{
				name:          "5. Command too long",
				command:       []string{"ZREMRANGEBYSCORE", "ZremRangeByScoreKey5", "4", "5", "8"},
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
						case *sorted_set.SortedSet:
							command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(key)}
							for _, member := range value.(*sorted_set.SortedSet).GetAll() {
								command = append(command, []resp.Value{
									resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
									resp.StringValue(string(member.Value)),
								}...)
							}
							expected = strconv.Itoa(value.(*sorted_set.SortedSet).Cardinality())
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
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), res.Error().Error())
					}
					return
				}

				if res.Integer() != test.expectedResponse {
					t.Errorf("expected response array of length %d, got %d", test.expectedResponse, res.Integer())
				}

				// Check if the resulting sorted set has the expected members/scores
				for key, expectedSortedSet := range test.expectedValues {
					if expectedSortedSet == nil {
						continue
					}

					if err = client.WriteArray([]resp.Value{
						resp.StringValue("ZRANGE"),
						resp.StringValue(key),
						resp.StringValue("-inf"),
						resp.StringValue("+inf"),
						resp.StringValue("BYSCORE"),
						resp.StringValue("WITHSCORES"),
					}); err != nil {
						t.Error(err)
					}

					res, _, err = client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if len(res.Array()) != expectedSortedSet.Cardinality() {
						t.Errorf("expected resulting set %s to have cardinality %d, got %d",
							key, expectedSortedSet.Cardinality(), len(res.Array()))
					}

					for _, member := range res.Array() {
						value := sorted_set.Value(member.Array()[0].String())
						score := sorted_set.Score(member.Array()[1].Float())
						if !expectedSortedSet.Contains(value) {
							t.Errorf("unexpected value %s in resulting sorted set", value)
						}
						if expectedSortedSet.Get(value).Score != score {
							t.Errorf("expected value %s to have score %v, got %v",
								value, expectedSortedSet.Get(value).Score, score)
						}
					}
				}
			})
		}
	})

	t.Run("Test_HandleZREMRANGEBYRANK", func(t *testing.T) {
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
			expectedValues   map[string]*sorted_set.SortedSet
			expectedResponse int
			expectedError    error
		}{
			{
				name: "1. Successfully remove multiple elements within range",
				presetValues: map[string]interface{}{
					"ZremRangeByRankKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
					}),
				},
				command: []string{"ZREMRANGEBYRANK", "ZremRangeByRankKey1", "0", "5"},
				expectedValues: map[string]*sorted_set.SortedSet{
					"ZremRangeByRankKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
					}),
				},
				expectedResponse: 6,
				expectedError:    nil,
			},
			{
				name: "2. Establish boundaries from the end of the set when negative boundaries are provided",
				presetValues: map[string]interface{}{
					"ZremRangeByRankKey2": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
					}),
				},
				command: []string{"ZREMRANGEBYRANK", "ZremRangeByRankKey2", "-6", "-3"},
				expectedValues: map[string]*sorted_set.SortedSet{
					"ZremRangeByRankKey2": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
					}),
				},
				expectedResponse: 4,
				expectedError:    nil,
			},
			{
				name:             "3. If key does not exist, return 0",
				presetValues:     nil,
				command:          []string{"ZREMRANGEBYRANK", "ZremRangeByRankKey3", "2", "4"},
				expectedValues:   nil,
				expectedResponse: 0,
				expectedError:    nil,
			},
			{
				name: "4. Return error key is not a sorted set",
				presetValues: map[string]interface{}{
					"ZremRangeByRankKey3": "Default value",
				},
				command:       []string{"ZREMRANGEBYRANK", "ZremRangeByRankKey3", "4", "4"},
				expectedError: errors.New("value at ZremRangeByRankKey3 is not a sorted set"),
			},
			{
				name: "5. Return error when start index is out of bounds",
				presetValues: map[string]interface{}{
					"ZremRangeByRankKey5": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
					}),
				},
				command:          []string{"ZREMRANGEBYRANK", "ZremRangeByRankKey5", "-12", "5"},
				expectedValues:   nil,
				expectedResponse: 0,
				expectedError:    errors.New("indices out of bounds"),
			},
			{
				name: "6. Return error when end index is out of bounds",
				presetValues: map[string]interface{}{
					"ZremRangeByRankKey6": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
					}),
				},
				command:          []string{"ZREMRANGEBYRANK", "ZremRangeByRankKey6", "0", "11"},
				expectedValues:   nil,
				expectedResponse: 0,
				expectedError:    errors.New("indices out of bounds"),
			},
			{
				name:          "7. Command too short",
				command:       []string{"ZREMRANGEBYRANK", "ZremRangeByRankKey4", "3"},
				expectedError: errors.New(constants.WrongArgsResponse),
			},
			{
				name:          "8. Command too long",
				command:       []string{"ZREMRANGEBYRANK", "ZremRangeByRankKey7", "4", "5", "8"},
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
						case *sorted_set.SortedSet:
							command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(key)}
							for _, member := range value.(*sorted_set.SortedSet).GetAll() {
								command = append(command, []resp.Value{
									resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
									resp.StringValue(string(member.Value)),
								}...)
							}
							expected = strconv.Itoa(value.(*sorted_set.SortedSet).Cardinality())
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
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), res.Error().Error())
					}
					return
				}

				if res.Integer() != test.expectedResponse {
					t.Errorf("expected response array of length %d, got %d", test.expectedResponse, res.Integer())
				}

				// Check if the resulting sorted set has the expected members/scores
				for key, expectedSortedSet := range test.expectedValues {
					if expectedSortedSet == nil {
						continue
					}

					if err = client.WriteArray([]resp.Value{
						resp.StringValue("ZRANGE"),
						resp.StringValue(key),
						resp.StringValue("-inf"),
						resp.StringValue("+inf"),
						resp.StringValue("BYSCORE"),
						resp.StringValue("WITHSCORES"),
					}); err != nil {
						t.Error(err)
					}

					res, _, err = client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if len(res.Array()) != expectedSortedSet.Cardinality() {
						t.Errorf("expected resulting set %s to have cardinality %d, got %d",
							key, expectedSortedSet.Cardinality(), len(res.Array()))
					}

					for _, member := range res.Array() {
						value := sorted_set.Value(member.Array()[0].String())
						score := sorted_set.Score(member.Array()[1].Float())
						if !expectedSortedSet.Contains(value) {
							t.Errorf("unexpected value %s in resulting sorted set", value)
						}
						if expectedSortedSet.Get(value).Score != score {
							t.Errorf("expected value %s to have score %v, got %v",
								value, expectedSortedSet.Get(value).Score, score)
						}
					}
				}
			})
		}
	})

	t.Run("Test_HandleZREMRANGEBYLEX", func(t *testing.T) {
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
			expectedValues   map[string]*sorted_set.SortedSet
			expectedResponse int
			expectedError    error
		}{
			{
				name: "1. Successfully remove multiple elements with scores inside the provided range",
				presetValues: map[string]interface{}{
					"ZremRangeByLexKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "a", Score: 1}, {Value: "b", Score: 1},
						{Value: "c", Score: 1}, {Value: "d", Score: 1},
						{Value: "e", Score: 1}, {Value: "f", Score: 1},
						{Value: "g", Score: 1}, {Value: "h", Score: 1},
						{Value: "i", Score: 1}, {Value: "j", Score: 1},
					}),
				},
				command: []string{"ZREMRANGEBYLEX", "ZremRangeByLexKey1", "a", "d"},
				expectedValues: map[string]*sorted_set.SortedSet{
					"ZremRangeByLexKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "e", Score: 1}, {Value: "f", Score: 1},
						{Value: "g", Score: 1}, {Value: "h", Score: 1},
						{Value: "i", Score: 1}, {Value: "j", Score: 1},
					}),
				},
				expectedResponse: 4,
				expectedError:    nil,
			},
			{
				name: "2. Return 0 if the members do not have the same score",
				presetValues: map[string]interface{}{
					"ZremRangeByLexKey2": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "a", Score: 1}, {Value: "b", Score: 2},
						{Value: "c", Score: 3}, {Value: "d", Score: 4},
						{Value: "e", Score: 5}, {Value: "f", Score: 6},
						{Value: "g", Score: 7}, {Value: "h", Score: 8},
						{Value: "i", Score: 9}, {Value: "j", Score: 10},
					}),
				},
				command: []string{"ZREMRANGEBYLEX", "ZremRangeByLexKey2", "d", "g"},
				expectedValues: map[string]*sorted_set.SortedSet{
					"ZremRangeByLexKey2": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "a", Score: 1}, {Value: "b", Score: 2},
						{Value: "c", Score: 3}, {Value: "d", Score: 4},
						{Value: "e", Score: 5}, {Value: "f", Score: 6},
						{Value: "g", Score: 7}, {Value: "h", Score: 8},
						{Value: "i", Score: 9}, {Value: "j", Score: 10},
					}),
				},
				expectedResponse: 0,
				expectedError:    nil,
			},
			{
				name:             "3. If key does not exist, return 0",
				presetValues:     nil,
				command:          []string{"ZREMRANGEBYLEX", "ZremRangeByLexKey3", "2", "4"},
				expectedValues:   nil,
				expectedResponse: 0,
				expectedError:    nil,
			},
			{
				name: "4. Return error key is not a sorted set",
				presetValues: map[string]interface{}{
					"ZremRangeByLexKey3": "Default value",
				},
				command:       []string{"ZREMRANGEBYLEX", "ZremRangeByLexKey3", "a", "d"},
				expectedError: errors.New("value at ZremRangeByLexKey3 is not a sorted set"),
			},
			{
				name:          "5. Command too short",
				command:       []string{"ZREMRANGEBYLEX", "ZremRangeByLexKey4", "a"},
				expectedError: errors.New(constants.WrongArgsResponse),
			},
			{
				name:          "6. Command too long",
				command:       []string{"ZREMRANGEBYLEX", "ZremRangeByLexKey5", "a", "b", "c"},
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
						case *sorted_set.SortedSet:
							command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(key)}
							for _, member := range value.(*sorted_set.SortedSet).GetAll() {
								command = append(command, []resp.Value{
									resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
									resp.StringValue(string(member.Value)),
								}...)
							}
							expected = strconv.Itoa(value.(*sorted_set.SortedSet).Cardinality())
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
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), res.Error().Error())
					}
					return
				}

				if res.Integer() != test.expectedResponse {
					t.Errorf("expected response array of length %d, got %d", test.expectedResponse, res.Integer())
				}

				// Check if the resulting sorted set has the expected members/scores
				for key, expectedSortedSet := range test.expectedValues {
					if expectedSortedSet == nil {
						continue
					}

					if err = client.WriteArray([]resp.Value{
						resp.StringValue("ZRANGE"),
						resp.StringValue(key),
						resp.StringValue("-inf"),
						resp.StringValue("+inf"),
						resp.StringValue("BYSCORE"),
						resp.StringValue("WITHSCORES"),
					}); err != nil {
						t.Error(err)
					}

					res, _, err = client.ReadValue()
					if err != nil {
						t.Error(err)
					}

					if len(res.Array()) != expectedSortedSet.Cardinality() {
						t.Errorf("expected resulting set %s to have cardinality %d, got %d",
							key, expectedSortedSet.Cardinality(), len(res.Array()))
					}

					for _, member := range res.Array() {
						value := sorted_set.Value(member.Array()[0].String())
						score := sorted_set.Score(member.Array()[1].Float())
						if !expectedSortedSet.Contains(value) {
							t.Errorf("unexpected value %s in resulting sorted set", value)
						}
						if expectedSortedSet.Get(value).Score != score {
							t.Errorf("expected value %s to have score %v, got %v",
								value, expectedSortedSet.Get(value).Score, score)
						}
					}
				}
			})
		}
	})

	t.Run("Test_HandleZRANGE", func(t *testing.T) {
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
			expectedResponse [][]string
			expectedError    error
		}{
			{
				name: "1. Get elements withing score range without score.",
				presetValues: map[string]interface{}{
					"ZrangeKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
				},
				command:          []string{"ZRANGE", "ZrangeKey1", "3", "7", "BYSCORE"},
				expectedResponse: [][]string{{"three"}, {"four"}, {"five"}, {"six"}, {"seven"}},
				expectedError:    nil,
			},
			{
				name: "2. Get elements within score range with score.",
				presetValues: map[string]interface{}{
					"ZrangeKey2": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
				},
				command: []string{"ZRANGE", "ZrangeKey2", "3", "7", "BYSCORE", "WITHSCORES"},
				expectedResponse: [][]string{
					{"three", "3"}, {"four", "4"}, {"five", "5"},
					{"six", "6"}, {"seven", "7"}},
				expectedError: nil,
			},
			{
				// 3. Get elements within score range with offset and limit.
				// Offset and limit are in where we start and stop counting in the original sorted set (NOT THE RESULT).
				name: "3. Get elements within score range with offset and limit.",
				presetValues: map[string]interface{}{
					"ZrangeKey3": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
				},
				command:          []string{"ZRANGE", "ZrangeKey3", "3", "7", "BYSCORE", "WITHSCORES", "LIMIT", "2", "4"},
				expectedResponse: [][]string{{"three", "3"}, {"four", "4"}, {"five", "5"}},
				expectedError:    nil,
			},
			{
				// 4. Get elements within score range with offset and limit + reverse the results.
				// Offset and limit are in where we start and stop counting in the original sorted set (NOT THE RESULT).
				// REV reverses the original set before getting the range.
				name: "4. Get elements within score range with offset and limit + reverse the results.",
				presetValues: map[string]interface{}{
					"ZrangeKey4": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
				},
				command:          []string{"ZRANGE", "ZrangeKey4", "3", "7", "BYSCORE", "WITHSCORES", "LIMIT", "2", "4", "REV"},
				expectedResponse: [][]string{{"six", "6"}, {"five", "5"}, {"four", "4"}},
				expectedError:    nil,
			},
			{
				name: "5. Get elements within lex range without score.",
				presetValues: map[string]interface{}{
					"ZrangeKey5": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "a", Score: 1}, {Value: "e", Score: 1},
						{Value: "b", Score: 1}, {Value: "f", Score: 1},
						{Value: "c", Score: 1}, {Value: "g", Score: 1},
						{Value: "d", Score: 1}, {Value: "h", Score: 1},
					}),
				},
				command:          []string{"ZRANGE", "ZrangeKey5", "c", "g", "BYLEX"},
				expectedResponse: [][]string{{"c"}, {"d"}, {"e"}, {"f"}, {"g"}},
				expectedError:    nil,
			},
			{
				name: "6. Get elements within lex range with score.",
				presetValues: map[string]interface{}{
					"ZrangeKey6": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "a", Score: 1}, {Value: "e", Score: 1},
						{Value: "b", Score: 1}, {Value: "f", Score: 1},
						{Value: "c", Score: 1}, {Value: "g", Score: 1},
						{Value: "d", Score: 1}, {Value: "h", Score: 1},
					}),
				},
				command: []string{"ZRANGE", "ZrangeKey6", "a", "f", "BYLEX", "WITHSCORES"},
				expectedResponse: [][]string{
					{"a", "1"}, {"b", "1"}, {"c", "1"},
					{"d", "1"}, {"e", "1"}, {"f", "1"}},
				expectedError: nil,
			},
			{
				// 7. Get elements within lex range with offset and limit.
				// Offset and limit are in where we start and stop counting in the original sorted set (NOT THE RESULT).
				name: "7. Get elements within lex range with offset and limit.",
				presetValues: map[string]interface{}{
					"ZrangeKey7": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "a", Score: 1}, {Value: "b", Score: 1},
						{Value: "c", Score: 1}, {Value: "d", Score: 1},
						{Value: "e", Score: 1}, {Value: "f", Score: 1},
						{Value: "g", Score: 1}, {Value: "h", Score: 1},
					}),
				},
				command:          []string{"ZRANGE", "ZrangeKey7", "a", "h", "BYLEX", "WITHSCORES", "LIMIT", "2", "4"},
				expectedResponse: [][]string{{"c", "1"}, {"d", "1"}, {"e", "1"}},
				expectedError:    nil,
			},
			{
				// 8. Get elements within lex range with offset and limit + reverse the results.
				// Offset and limit are in where we start and stop counting in the original sorted set (NOT THE RESULT).
				// REV reverses the original set before getting the range.
				name: "8. Get elements within lex range with offset and limit + reverse the results.",
				presetValues: map[string]interface{}{
					"ZrangeKey8": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "a", Score: 1}, {Value: "b", Score: 1},
						{Value: "c", Score: 1}, {Value: "d", Score: 1},
						{Value: "e", Score: 1}, {Value: "f", Score: 1},
						{Value: "g", Score: 1}, {Value: "h", Score: 1},
					}),
				},
				command:          []string{"ZRANGE", "ZrangeKey8", "a", "h", "BYLEX", "WITHSCORES", "LIMIT", "2", "4", "REV"},
				expectedResponse: [][]string{{"f", "1"}, {"e", "1"}, {"d", "1"}},
				expectedError:    nil,
			},
			{
				name: "9. Return an empty slice when we use BYLEX while elements have different scores",
				presetValues: map[string]interface{}{
					"ZrangeKey9": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "a", Score: 1}, {Value: "b", Score: 5},
						{Value: "c", Score: 2}, {Value: "d", Score: 6},
						{Value: "e", Score: 3}, {Value: "f", Score: 7},
						{Value: "g", Score: 4}, {Value: "h", Score: 8},
					}),
				},
				command:          []string{"ZRANGE", "ZrangeKey9", "a", "h", "BYLEX", "WITHSCORES", "LIMIT", "2", "4"},
				expectedResponse: [][]string{},
				expectedError:    nil,
			},
			{
				name:             "10. Throw error when limit does not provide both offset and limit",
				presetValues:     nil,
				command:          []string{"ZRANGE", "ZrangeKey10", "a", "h", "BYLEX", "WITHSCORES", "LIMIT", "2"},
				expectedResponse: [][]string{},
				expectedError:    errors.New("limit should contain offset and count as integers"),
			},
			{
				name:             "11. Throw error when offset is not a valid integer",
				presetValues:     nil,
				command:          []string{"ZRANGE", "ZrangeKey11", "a", "h", "BYLEX", "WITHSCORES", "LIMIT", "offset", "4"},
				expectedResponse: [][]string{},
				expectedError:    errors.New("limit offset must be integer"),
			},
			{
				name:             "12. Throw error when limit is not a valid integer",
				presetValues:     nil,
				command:          []string{"ZRANGE", "ZrangeKey12", "a", "h", "BYLEX", "WITHSCORES", "LIMIT", "4", "limit"},
				expectedResponse: [][]string{},
				expectedError:    errors.New("limit count must be integer"),
			},
			{
				name:             "13. Throw error when offset is negative",
				presetValues:     nil,
				command:          []string{"ZRANGE", "ZrangeKey13", "a", "h", "BYLEX", "WITHSCORES", "LIMIT", "-4", "9"},
				expectedResponse: [][]string{},
				expectedError:    errors.New("limit offset must be >= 0"),
			},
			{
				name: "14. Throw error when the key does not hold a sorted set",
				presetValues: map[string]interface{}{
					"ZrangeKey14": "Default value",
				},
				command:          []string{"ZRANGE", "ZrangeKey14", "a", "h", "BYLEX", "WITHSCORES", "LIMIT", "2", "4"},
				expectedResponse: [][]string{},
				expectedError:    errors.New("value at ZrangeKey14 is not a sorted set"),
			},
			{
				name:             "15. Command too short",
				presetValues:     nil,
				command:          []string{"ZRANGE", "ZrangeKey15", "1"},
				expectedResponse: [][]string{},
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "16. Command too long",
				presetValues:     nil,
				command:          []string{"ZRANGE", "ZrangeKey16", "a", "h", "BYLEX", "WITHSCORES", "LIMIT", "-4", "9", "REV", "WITHSCORES"},
				expectedResponse: [][]string{},
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
						case *sorted_set.SortedSet:
							command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(key)}
							for _, member := range value.(*sorted_set.SortedSet).GetAll() {
								command = append(command, []resp.Value{
									resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
									resp.StringValue(string(member.Value)),
								}...)
							}
							expected = strconv.Itoa(value.(*sorted_set.SortedSet).Cardinality())
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
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), res.Error().Error())
					}
					return
				}

				if len(res.Array()) != len(test.expectedResponse) {
					t.Errorf("expected response array of length %d, got %d", len(test.expectedResponse), len(res.Array()))
				}

				for _, item := range res.Array() {
					value := item.Array()[0].String()
					score := func() string {
						if len(item.Array()) == 2 {
							return item.Array()[1].String()
						}
						return ""
					}()
					if !slices.ContainsFunc(test.expectedResponse, func(expected []string) bool {
						return expected[0] == value
					}) {
						t.Errorf("unexpected member \"%s\" in response", value)
					}
					if score != "" {
						for _, expected := range test.expectedResponse {
							if expected[0] == value && expected[1] != score {
								t.Errorf("expected score for member \"%s\" to be %s, got %s", value, expected[1], score)
							}
						}
					}
				}
			})
		}
	})

	t.Run("Test_HandleZRANGESTORE", func(t *testing.T) {
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
			expectedValue    *sorted_set.SortedSet
			expectedResponse int
			expectedError    error
		}{
			{
				name: "1. Get elements withing score range without score.",
				presetValues: map[string]interface{}{
					"ZrangeStoreKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
				},
				destination:      "ZrangeStoreDestinationKey1",
				command:          []string{"ZRANGESTORE", "ZrangeStoreDestinationKey1", "ZrangeStoreKey1", "3", "7", "BYSCORE"},
				expectedResponse: 5,
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "three", Score: 3}, {Value: "four", Score: 4}, {Value: "five", Score: 5},
					{Value: "six", Score: 6}, {Value: "seven", Score: 7},
				}),
				expectedError: nil,
			},
			{
				name: "2. Get elements within score range with score.",
				presetValues: map[string]interface{}{
					"ZrangeStoreKey2": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
				},
				destination:      "ZrangeStoreDestinationKey2",
				command:          []string{"ZRANGESTORE", "ZrangeStoreDestinationKey2", "ZrangeStoreKey2", "3", "7", "BYSCORE", "WITHSCORES"},
				expectedResponse: 5,
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "three", Score: 3}, {Value: "four", Score: 4}, {Value: "five", Score: 5},
					{Value: "six", Score: 6}, {Value: "seven", Score: 7},
				}),
				expectedError: nil,
			},
			{
				// 3. Get elements within score range with offset and limit.
				// Offset and limit are in where we start and stop counting in the original sorted set (NOT THE RESULT).
				name: "3. Get elements within score range with offset and limit.",
				presetValues: map[string]interface{}{
					"ZrangeStoreKey3": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
				},
				destination:      "ZrangeStoreDestinationKey3",
				command:          []string{"ZRANGESTORE", "ZrangeStoreDestinationKey3", "ZrangeStoreKey3", "3", "7", "BYSCORE", "WITHSCORES", "LIMIT", "2", "4"},
				expectedResponse: 3,
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "three", Score: 3}, {Value: "four", Score: 4}, {Value: "five", Score: 5},
				}),
				expectedError: nil,
			},
			{
				// 4. Get elements within score range with offset and limit + reverse the results.
				// Offset and limit are in where we start and stop counting in the original sorted set (NOT THE RESULT).
				// REV reverses the original set before getting the range.
				name: "4. Get elements within score range with offset and limit + reverse the results.",
				presetValues: map[string]interface{}{
					"ZrangeStoreKey4": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
				},
				destination:      "ZrangeStoreDestinationKey4",
				command:          []string{"ZRANGESTORE", "ZrangeStoreDestinationKey4", "ZrangeStoreKey4", "3", "7", "BYSCORE", "WITHSCORES", "LIMIT", "2", "4", "REV"},
				expectedResponse: 3,
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "six", Score: 6}, {Value: "five", Score: 5}, {Value: "four", Score: 4},
				}),
				expectedError: nil,
			},
			{
				name: "5. Get elements within lex range without score.",
				presetValues: map[string]interface{}{
					"ZrangeStoreKey5": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "a", Score: 1}, {Value: "e", Score: 1},
						{Value: "b", Score: 1}, {Value: "f", Score: 1},
						{Value: "c", Score: 1}, {Value: "g", Score: 1},
						{Value: "d", Score: 1}, {Value: "h", Score: 1},
					}),
				},
				destination:      "ZrangeStoreDestinationKey5",
				command:          []string{"ZRANGESTORE", "ZrangeStoreDestinationKey5", "ZrangeStoreKey5", "c", "g", "BYLEX"},
				expectedResponse: 5,
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "c", Score: 1}, {Value: "d", Score: 1}, {Value: "e", Score: 1},
					{Value: "f", Score: 1}, {Value: "g", Score: 1},
				}),
				expectedError: nil,
			},
			{
				name: "6. Get elements within lex range with score.",
				presetValues: map[string]interface{}{
					"ZrangeStoreKey6": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "a", Score: 1}, {Value: "e", Score: 1},
						{Value: "b", Score: 1}, {Value: "f", Score: 1},
						{Value: "c", Score: 1}, {Value: "g", Score: 1},
						{Value: "d", Score: 1}, {Value: "h", Score: 1},
					}),
				},
				destination:      "ZrangeStoreDestinationKey6",
				command:          []string{"ZRANGESTORE", "ZrangeStoreDestinationKey6", "ZrangeStoreKey6", "a", "f", "BYLEX", "WITHSCORES"},
				expectedResponse: 6,
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "a", Score: 1}, {Value: "b", Score: 1}, {Value: "c", Score: 1},
					{Value: "d", Score: 1}, {Value: "e", Score: 1}, {Value: "f", Score: 1},
				}),
				expectedError: nil,
			},
			{
				// 7. Get elements within lex range with offset and limit.
				// Offset and limit are in where we start and stop counting in the original sorted set (NOT THE RESULT).
				name: "7. Get elements within lex range with offset and limit.",
				presetValues: map[string]interface{}{
					"ZrangeStoreKey7": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "a", Score: 1}, {Value: "b", Score: 1},
						{Value: "c", Score: 1}, {Value: "d", Score: 1},
						{Value: "e", Score: 1}, {Value: "f", Score: 1},
						{Value: "g", Score: 1}, {Value: "h", Score: 1},
					}),
				},
				destination:      "ZrangeStoreDestinationKey7",
				command:          []string{"ZRANGESTORE", "ZrangeStoreDestinationKey7", "ZrangeStoreKey7", "a", "h", "BYLEX", "WITHSCORES", "LIMIT", "2", "4"},
				expectedResponse: 3,
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "c", Score: 1}, {Value: "d", Score: 1}, {Value: "e", Score: 1},
				}),
				expectedError: nil,
			},
			{
				// 8. Get elements within lex range with offset and limit + reverse the results.
				// Offset and limit are in where we start and stop counting in the original sorted set (NOT THE RESULT).
				// REV reverses the original set before getting the range.
				name: "8. Get elements within lex range with offset and limit + reverse the results.",
				presetValues: map[string]interface{}{
					"ZrangeStoreKey8": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "a", Score: 1}, {Value: "b", Score: 1},
						{Value: "c", Score: 1}, {Value: "d", Score: 1},
						{Value: "e", Score: 1}, {Value: "f", Score: 1},
						{Value: "g", Score: 1}, {Value: "h", Score: 1},
					}),
				},
				destination:      "ZrangeStoreDestinationKey8",
				command:          []string{"ZRANGESTORE", "ZrangeStoreDestinationKey8", "ZrangeStoreKey8", "a", "h", "BYLEX", "WITHSCORES", "LIMIT", "2", "4", "REV"},
				expectedResponse: 3,
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "f", Score: 1}, {Value: "e", Score: 1}, {Value: "d", Score: 1},
				}),
				expectedError: nil,
			},
			{
				name: "9. Return an empty slice when we use BYLEX while elements have different scores",
				presetValues: map[string]interface{}{
					"ZrangeStoreKey9": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "a", Score: 1}, {Value: "b", Score: 5},
						{Value: "c", Score: 2}, {Value: "d", Score: 6},
						{Value: "e", Score: 3}, {Value: "f", Score: 7},
						{Value: "g", Score: 4}, {Value: "h", Score: 8},
					}),
				},
				destination:      "ZrangeStoreDestinationKey9",
				command:          []string{"ZRANGESTORE", "ZrangeStoreDestinationKey9", "ZrangeStoreKey9", "a", "h", "BYLEX", "WITHSCORES", "LIMIT", "2", "4"},
				expectedResponse: 0,
				expectedValue:    nil,
				expectedError:    nil,
			},
			{
				name:             "10. Throw error when limit does not provide both offset and limit",
				presetValues:     nil,
				command:          []string{"ZRANGESTORE", "ZrangeStoreDestinationKey10", "ZrangeStoreKey10", "a", "h", "BYLEX", "WITHSCORES", "LIMIT", "2"},
				expectedResponse: 0,
				expectedError:    errors.New("limit should contain offset and count as integers"),
			},
			{
				name:             "11. Throw error when offset is not a valid integer",
				presetValues:     nil,
				command:          []string{"ZRANGESTORE", "ZrangeStoreDestinationKey11", "ZrangeStoreKey11", "a", "h", "BYLEX", "WITHSCORES", "LIMIT", "offset", "4"},
				expectedResponse: 0,
				expectedError:    errors.New("limit offset must be integer"),
			},
			{
				name:             "12. Throw error when limit is not a valid integer",
				presetValues:     nil,
				command:          []string{"ZRANGESTORE", "ZrangeStoreDestinationKey12", "ZrangeStoreKey12", "a", "h", "BYLEX", "WITHSCORES", "LIMIT", "4", "limit"},
				expectedResponse: 0,
				expectedError:    errors.New("limit count must be integer"),
			},
			{
				name:             "13. Throw error when offset is negative",
				presetValues:     nil,
				command:          []string{"ZRANGESTORE", "ZrangeStoreDestinationKey13", "ZrangeStoreKey13", "a", "h", "BYLEX", "WITHSCORES", "LIMIT", "-4", "9"},
				expectedResponse: 0,
				expectedError:    errors.New("limit offset must be >= 0"),
			},
			{
				name: "14. Throw error when the key does not hold a sorted set",
				presetValues: map[string]interface{}{
					"ZrangeStoreKey14": "Default value",
				},
				command:          []string{"ZRANGESTORE", "ZrangeStoreDestinationKey14", "ZrangeStoreKey14", "a", "h", "BYLEX", "WITHSCORES", "LIMIT", "2", "4"},
				expectedResponse: 0,
				expectedError:    errors.New("value at ZrangeStoreKey14 is not a sorted set"),
			},
			{
				name:             "15. Command too short",
				presetValues:     nil,
				command:          []string{"ZRANGESTORE", "ZrangeStoreKey15", "1"},
				expectedResponse: 0,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:             "16 Command too long",
				presetValues:     nil,
				command:          []string{"ZRANGESTORE", "ZrangeStoreDestinationKey16", "ZrangeStoreKey16", "a", "h", "BYLEX", "WITHSCORES", "LIMIT", "-4", "9", "REV", "WITHSCORES"},
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
						case *sorted_set.SortedSet:
							command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(key)}
							for _, member := range value.(*sorted_set.SortedSet).GetAll() {
								command = append(command, []resp.Value{
									resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
									resp.StringValue(string(member.Value)),
								}...)
							}
							expected = strconv.Itoa(value.(*sorted_set.SortedSet).Cardinality())
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
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), res.Error().Error())
					}
					return
				}

				if res.Integer() != test.expectedResponse {
					t.Errorf("expected response %d, got %d", test.expectedResponse, res.Integer())
				}

				// Check if the resulting sorted set has the expected members/scores
				if test.expectedValue == nil {
					return
				}

				if err = client.WriteArray([]resp.Value{
					resp.StringValue("ZRANGE"),
					resp.StringValue(test.destination),
					resp.StringValue("-inf"),
					resp.StringValue("+inf"),
					resp.StringValue("BYSCORE"),
					resp.StringValue("WITHSCORES"),
				}); err != nil {
					t.Error(err)
				}

				res, _, err = client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if len(res.Array()) != test.expectedValue.Cardinality() {
					t.Errorf("expected resulting set %s to have cardinality %d, got %d",
						test.destination, test.expectedValue.Cardinality(), len(res.Array()))
				}

				for _, member := range res.Array() {
					value := sorted_set.Value(member.Array()[0].String())
					score := sorted_set.Score(member.Array()[1].Float())
					if !test.expectedValue.Contains(value) {
						t.Errorf("unexpected value %s in resulting sorted set", value)
					}
					if test.expectedValue.Get(value).Score != score {
						t.Errorf("expected value %s to have score %v, got %v", value, test.expectedValue.Get(value).Score, score)
					}
				}
			})
		}
	})

	t.Run("Test_HandleZINTER", func(t *testing.T) {
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
			expectedResponse [][]string
			expectedError    error
		}{
			{
				name: "1. Get the intersection between 2 sorted sets.",
				presetValues: map[string]interface{}{
					"ZinterKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5},
					}),
					"ZinterKey2": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
				},
				command:          []string{"ZINTER", "ZinterKey1", "ZinterKey2"},
				expectedResponse: [][]string{{"three"}, {"four"}, {"five"}},
				expectedError:    nil,
			},
			{
				// 2. Get the intersection between 3 sorted sets with scores.
				// By default, the SUM aggregate will be used.
				name: "2. Get the intersection between 3 sorted sets with scores.",
				presetValues: map[string]interface{}{
					"ZinterKey3": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZinterKey4": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 8},
					}),
					"ZinterKey5": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "eight", Score: 8},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				command:          []string{"ZINTER", "ZinterKey3", "ZinterKey4", "ZinterKey5", "WITHSCORES"},
				expectedResponse: [][]string{{"one", "3"}, {"eight", "24"}},
				expectedError:    nil,
			},
			{
				// 3. Get the intersection between 3 sorted sets with scores.
				// Use MIN aggregate.
				name: "3. Get the intersection between 3 sorted sets with scores.",
				presetValues: map[string]interface{}{
					"ZinterKey6": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 100}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZinterKey7": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 80},
					}),
					"ZinterKey8": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1000}, {Value: "eight", Score: 800},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				command:          []string{"ZINTER", "ZinterKey6", "ZinterKey7", "ZinterKey8", "WITHSCORES", "AGGREGATE", "MIN"},
				expectedResponse: [][]string{{"one", "1"}, {"eight", "8"}},
				expectedError:    nil,
			},
			{
				// 4. Get the intersection between 3 sorted sets with scores.
				// Use MAX aggregate.
				name: "4. Get the intersection between 3 sorted sets with scores.",
				presetValues: map[string]interface{}{
					"ZinterKey9": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 100}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZinterKey10": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 80},
					}),
					"ZinterKey11": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1000}, {Value: "eight", Score: 800},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				command:          []string{"ZINTER", "ZinterKey9", "ZinterKey10", "ZinterKey11", "WITHSCORES", "AGGREGATE", "MAX"},
				expectedResponse: [][]string{{"one", "1000"}, {"eight", "800"}},
				expectedError:    nil,
			},
			{
				// 5. Get the intersection between 3 sorted sets with scores.
				// Use SUM aggregate with weights modifier.
				name: "5. Get the intersection between 3 sorted sets with scores.",
				presetValues: map[string]interface{}{
					"ZinterKey12": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 100}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZinterKey13": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 80},
					}),
					"ZinterKey14": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1000}, {Value: "eight", Score: 800},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				command:          []string{"ZINTER", "ZinterKey12", "ZinterKey13", "ZinterKey14", "WITHSCORES", "AGGREGATE", "SUM", "WEIGHTS", "1", "5", "3"},
				expectedResponse: [][]string{{"one", "3105"}, {"eight", "2808"}},
				expectedError:    nil,
			},
			{
				// 6. Get the intersection between 3 sorted sets with scores.
				// Use MAX aggregate with added weights.
				name: "6. Get the intersection between 3 sorted sets with scores.",
				presetValues: map[string]interface{}{
					"ZinterKey15": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 100}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZinterKey16": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 80},
					}),
					"ZinterKey17": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1000}, {Value: "eight", Score: 800},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				command:          []string{"ZINTER", "ZinterKey15", "ZinterKey16", "ZinterKey17", "WITHSCORES", "AGGREGATE", "MAX", "WEIGHTS", "1", "5", "3"},
				expectedResponse: [][]string{{"one", "3000"}, {"eight", "2400"}},
				expectedError:    nil,
			},
			{
				// 7. Get the intersection between 3 sorted sets with scores.
				// Use MIN aggregate with added weights.
				name: "7. Get the intersection between 3 sorted sets with scores.",
				presetValues: map[string]interface{}{
					"ZinterKey18": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 100}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZinterKey19": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 80},
					}),
					"ZinterKey20": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1000}, {Value: "eight", Score: 800},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				command:          []string{"ZINTER", "ZinterKey18", "ZinterKey19", "ZinterKey20", "WITHSCORES", "AGGREGATE", "MIN", "WEIGHTS", "1", "5", "3"},
				expectedResponse: [][]string{{"one", "5"}, {"eight", "8"}},
				expectedError:    nil,
			},
			{
				name: "8. Throw an error if there are more weights than keys",
				presetValues: map[string]interface{}{
					"ZinterKey21": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZinterKey22": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
				},
				command:          []string{"ZINTER", "ZinterKey21", "ZinterKey22", "WEIGHTS", "1", "2", "3"},
				expectedResponse: nil,
				expectedError:    errors.New("number of weights should match number of keys"),
			},
			{
				name: "9. Throw an error if there are fewer weights than keys",
				presetValues: map[string]interface{}{
					"ZinterKey23": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZinterKey24": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
					}),
					"ZinterKey25": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
				},
				command:          []string{"ZINTER", "ZinterKey23", "ZinterKey24", "ZinterKey25", "WEIGHTS", "5", "4"},
				expectedResponse: nil,
				expectedError:    errors.New("number of weights should match number of keys"),
			},
			{
				name: "10. Throw an error if there are no keys provided",
				presetValues: map[string]interface{}{
					"ZinterKey26": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
					"ZinterKey27": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
					"ZinterKey28": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
				},
				command:          []string{"ZINTER", "WEIGHTS", "5", "4"},
				expectedResponse: nil,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name: "11. Throw an error if any of the provided keys are not sorted sets",
				presetValues: map[string]interface{}{
					"ZinterKey29": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZinterKey30": "Default value",
					"ZinterKey31": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
				},
				command:          []string{"ZINTER", "ZinterKey29", "ZinterKey30", "ZinterKey31"},
				expectedResponse: nil,
				expectedError:    errors.New("value at ZinterKey30 is not a sorted set"),
			},
			{
				name: "12. If any of the keys does not exist, return an empty array.",
				presetValues: map[string]interface{}{
					"ZinterKey32": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11},
					}),
					"ZinterKey33": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				command:          []string{"ZINTER", "non-existent", "ZinterKey32", "ZinterKey33"},
				expectedResponse: [][]string{},
				expectedError:    nil,
			},
			{
				name:             "13. Command too short",
				command:          []string{"ZINTER"},
				expectedResponse: [][]string{},
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
						case *sorted_set.SortedSet:
							command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(key)}
							for _, member := range value.(*sorted_set.SortedSet).GetAll() {
								command = append(command, []resp.Value{
									resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
									resp.StringValue(string(member.Value)),
								}...)
							}
							expected = strconv.Itoa(value.(*sorted_set.SortedSet).Cardinality())
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
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), res.Error().Error())
					}
					return
				}

				if len(res.Array()) != len(test.expectedResponse) {
					t.Errorf("expected response array of length %d, got %d", len(test.expectedResponse), len(res.Array()))
				}

				for _, item := range res.Array() {
					value := item.Array()[0].String()
					score := func() string {
						if len(item.Array()) == 2 {
							return item.Array()[1].String()
						}
						return ""
					}()
					if !slices.ContainsFunc(test.expectedResponse, func(expected []string) bool {
						return expected[0] == value
					}) {
						t.Errorf("unexpected member \"%s\" in response", value)
					}
					if score != "" {
						for _, expected := range test.expectedResponse {
							if expected[0] == value && expected[1] != score {
								t.Errorf("expected score for member \"%s\" to be %s, got %s", value, expected[1], score)
							}
						}
					}
				}
			})
		}
	})

	t.Run("Test_HandleZINTERSTORE", func(t *testing.T) {
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
			expectedValue    *sorted_set.SortedSet
			expectedResponse int
			expectedError    error
		}{
			{
				name: "1. Get the intersection between 2 sorted sets.",
				presetValues: map[string]interface{}{
					"ZinterStoreKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5},
					}),
					"ZinterStoreKey2": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
				},
				destination: "ZinterStoreDestinationKey1",
				command:     []string{"ZINTERSTORE", "ZinterStoreDestinationKey1", "ZinterStoreKey1", "ZinterStoreKey2"},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "three", Score: 6}, {Value: "four", Score: 8},
					{Value: "five", Score: 10},
				}),
				expectedResponse: 3,
				expectedError:    nil,
			},
			{
				// 2. Get the intersection between 3 sorted sets with scores.
				// By default, the SUM aggregate will be used.
				name: "2. Get the intersection between 3 sorted sets with scores.",
				presetValues: map[string]interface{}{
					"ZinterStoreKey3": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZinterStoreKey4": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 8},
					}),
					"ZinterStoreKey5": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "eight", Score: 8},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				destination: "ZinterStoreDestinationKey2",
				command: []string{
					"ZINTERSTORE", "ZinterStoreDestinationKey2", "ZinterStoreKey3", "ZinterStoreKey4", "ZinterStoreKey5", "WITHSCORES",
				},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 3}, {Value: "eight", Score: 24},
				}),
				expectedResponse: 2,
				expectedError:    nil,
			},
			{
				// 3. Get the intersection between 3 sorted sets with scores.
				// Use MIN aggregate.
				name: "3. Get the intersection between 3 sorted sets with scores.",
				presetValues: map[string]interface{}{
					"ZinterStoreKey6": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 100}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZinterStoreKey7": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 80},
					}),
					"ZinterStoreKey8": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1000}, {Value: "eight", Score: 800},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				destination: "ZinterStoreDestinationKey3",
				command:     []string{"ZINTERSTORE", "ZinterStoreDestinationKey3", "ZinterStoreKey6", "ZinterStoreKey7", "ZinterStoreKey8", "WITHSCORES", "AGGREGATE", "MIN"},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 1}, {Value: "eight", Score: 8},
				}),
				expectedResponse: 2,
				expectedError:    nil,
			},
			{
				// 4. Get the intersection between 3 sorted sets with scores.
				// Use MAX aggregate.
				name: "4. Get the intersection between 3 sorted sets with scores.",
				presetValues: map[string]interface{}{
					"ZinterStoreKey9": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 100}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZinterStoreKey10": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 80},
					}),
					"ZinterStoreKey11": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1000}, {Value: "eight", Score: 800},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				destination: "ZinterStoreDestinationKey4",
				command:     []string{"ZINTERSTORE", "ZinterStoreDestinationKey4", "ZinterStoreKey9", "ZinterStoreKey10", "ZinterStoreKey11", "WITHSCORES", "AGGREGATE", "MAX"},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 1000}, {Value: "eight", Score: 800},
				}),
				expectedResponse: 2,
				expectedError:    nil,
			},
			{
				// 5. Get the intersection between 3 sorted sets with scores.
				// Use SUM aggregate with weights modifier.
				name: "5. Get the intersection between 3 sorted sets with scores.",
				presetValues: map[string]interface{}{
					"ZinterStoreKey12": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 100}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZinterStoreKey13": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 80},
					}),
					"ZinterStoreKey14": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1000}, {Value: "eight", Score: 800},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				destination: "ZinterStoreDestinationKey5",
				command:     []string{"ZINTERSTORE", "ZinterStoreDestinationKey5", "ZinterStoreKey12", "ZinterStoreKey13", "ZinterStoreKey14", "WITHSCORES", "AGGREGATE", "SUM", "WEIGHTS", "1", "5", "3"},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 3105}, {Value: "eight", Score: 2808},
				}),
				expectedResponse: 2,
				expectedError:    nil,
			},
			{
				// 6. Get the intersection between 3 sorted sets with scores.
				// Use MAX aggregate with added weights.
				name: "6. Get the intersection between 3 sorted sets with scores.",
				presetValues: map[string]interface{}{
					"ZinterStoreKey15": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 100}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZinterStoreKey16": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 80},
					}),
					"ZinterStoreKey17": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1000}, {Value: "eight", Score: 800},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				destination: "ZinterStoreDestinationKey6",
				command:     []string{"ZINTERSTORE", "ZinterStoreDestinationKey6", "ZinterStoreKey15", "ZinterStoreKey16", "ZinterStoreKey17", "WITHSCORES", "AGGREGATE", "MAX", "WEIGHTS", "1", "5", "3"},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 3000}, {Value: "eight", Score: 2400},
				}),
				expectedResponse: 2,
				expectedError:    nil,
			},
			{
				// 7. Get the intersection between 3 sorted sets with scores.
				// Use MIN aggregate with added weights.
				name: "7. Get the intersection between 3 sorted sets with scores.",
				presetValues: map[string]interface{}{
					"ZinterStoreKey18": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 100}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZinterStoreKey19": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 80},
					}),
					"ZinterStoreKey20": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1000}, {Value: "eight", Score: 800},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				destination: "ZinterStoreDestinationKey7",
				command:     []string{"ZINTERSTORE", "ZinterStoreDestinationKey7", "ZinterStoreKey18", "ZinterStoreKey19", "ZinterStoreKey20", "WITHSCORES", "AGGREGATE", "MIN", "WEIGHTS", "1", "5", "3"},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 5}, {Value: "eight", Score: 8},
				}),
				expectedResponse: 2,
				expectedError:    nil,
			},
			{
				name: "8. Throw an error if there are more weights than keys",
				presetValues: map[string]interface{}{
					"ZinterStoreKey21": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZinterStoreKey22": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
				},
				command:          []string{"ZINTERSTORE", "ZinterStoreDestinationKey8", "ZinterStoreKey21", "ZinterStoreKey22", "WEIGHTS", "1", "2", "3"},
				expectedResponse: 0,
				expectedError:    errors.New("number of weights should match number of keys"),
			},
			{
				name: "9. Throw an error if there are fewer weights than keys",
				presetValues: map[string]interface{}{
					"ZinterStoreKey23": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZinterStoreKey24": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
					}),
					"ZinterStoreKey25": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
				},
				command:          []string{"ZINTERSTORE", "ZinterStoreDestinationKey9", "ZinterStoreKey23", "ZinterStoreKey24", "ZinterStoreKey25", "WEIGHTS", "5", "4"},
				expectedResponse: 0,
				expectedError:    errors.New("number of weights should match number of keys"),
			},
			{
				name: "10. Throw an error if there are no keys provided",
				presetValues: map[string]interface{}{
					"ZinterStoreKey26": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
					"ZinterStoreKey27": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
					"ZinterStoreKey28": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
				},
				command:          []string{"ZINTERSTORE", "WEIGHTS", "5", "4"},
				expectedResponse: 0,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name: "11. Throw an error if any of the provided keys are not sorted sets",
				presetValues: map[string]interface{}{
					"ZinterStoreKey29": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZinterStoreKey30": "Default value",
					"ZinterStoreKey31": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
				},
				command:          []string{"ZINTERSTORE", "ZinterStoreKey29", "ZinterStoreKey30", "ZinterStoreKey31"},
				expectedResponse: 0,
				expectedError:    errors.New("value at ZinterStoreKey30 is not a sorted set"),
			},
			{
				name: "12. If any of the keys does not exist, return an empty array.",
				presetValues: map[string]interface{}{
					"ZinterStoreKey32": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11},
					}),
					"ZinterStoreKey33": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				command:          []string{"ZINTERSTORE", "ZinterStoreDestinationKey12", "non-existent", "ZinterStoreKey32", "ZinterStoreKey33"},
				expectedResponse: 0,
				expectedError:    nil,
			},
			{
				name:             "13. Command too short",
				command:          []string{"ZINTERSTORE"},
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
						case *sorted_set.SortedSet:
							command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(key)}
							for _, member := range value.(*sorted_set.SortedSet).GetAll() {
								command = append(command, []resp.Value{
									resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
									resp.StringValue(string(member.Value)),
								}...)
							}
							expected = strconv.Itoa(value.(*sorted_set.SortedSet).Cardinality())
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
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), res.Error().Error())
					}
					return
				}

				if res.Integer() != test.expectedResponse {
					t.Errorf("expected response %d, got %d", test.expectedResponse, res.Integer())
				}

				// Check if the resulting sorted set has the expected members/scores
				if test.expectedValue == nil {
					return
				}

				if err = client.WriteArray([]resp.Value{
					resp.StringValue("ZRANGE"),
					resp.StringValue(test.destination),
					resp.StringValue("-inf"),
					resp.StringValue("+inf"),
					resp.StringValue("BYSCORE"),
					resp.StringValue("WITHSCORES"),
				}); err != nil {
					t.Error(err)
				}

				res, _, err = client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if len(res.Array()) != test.expectedValue.Cardinality() {
					t.Errorf("expected resulting set %s to have cardinality %d, got %d",
						test.destination, test.expectedValue.Cardinality(), len(res.Array()))
				}

				for _, member := range res.Array() {
					value := sorted_set.Value(member.Array()[0].String())
					score := sorted_set.Score(member.Array()[1].Float())
					if !test.expectedValue.Contains(value) {
						t.Errorf("unexpected value %s in resulting sorted set", value)
					}
					if test.expectedValue.Get(value).Score != score {
						t.Errorf("expected value %s to have score %v, got %v", value, test.expectedValue.Get(value).Score, score)
					}
				}
			})
		}
	})

	t.Run("Test_HandleZUNION", func(t *testing.T) {
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
			expectedResponse [][]string
			expectedError    error
		}{
			{
				name: "1. Get the union between 2 sorted sets.",
				presetValues: map[string]interface{}{
					"ZunionKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5},
					}),
					"ZunionKey2": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
				},
				command:          []string{"ZUNION", "ZunionKey1", "ZunionKey2"},
				expectedResponse: [][]string{{"one"}, {"two"}, {"three"}, {"four"}, {"five"}, {"six"}, {"seven"}, {"eight"}},
				expectedError:    nil,
			},
			{
				// 2. Get the union between 3 sorted sets with scores.
				// By default, the SUM aggregate will be used.
				name: "2. Get the union between 3 sorted sets with scores.",
				presetValues: map[string]interface{}{
					"ZunionKey3": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZunionKey4": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 8},
					}),
					"ZunionKey5": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "eight", Score: 8},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12}, {Value: "thirty-six", Score: 36},
					}),
				},
				command: []string{"ZUNION", "ZunionKey3", "ZunionKey4", "ZunionKey5", "WITHSCORES"},
				expectedResponse: [][]string{
					{"one", "3"}, {"two", "4"}, {"three", "3"}, {"four", "4"}, {"five", "5"}, {"six", "6"},
					{"seven", "7"}, {"eight", "24"}, {"nine", "9"}, {"ten", "10"}, {"eleven", "11"},
					{"twelve", "24"}, {"thirty-six", "72"},
				},
				expectedError: nil,
			},
			{
				// 3. Get the union between 3 sorted sets with scores.
				// Use MIN aggregate.
				name: "3. Get the union between 3 sorted sets with scores.",
				presetValues: map[string]interface{}{
					"ZunionKey6": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 100}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZunionKey7": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 80},
					}),
					"ZunionKey8": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1000}, {Value: "eight", Score: 800},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12}, {Value: "thirty-six", Score: 72},
					}),
				},
				command: []string{"ZUNION", "ZunionKey6", "ZunionKey7", "ZunionKey8", "WITHSCORES", "AGGREGATE", "MIN"},
				expectedResponse: [][]string{
					{"one", "1"}, {"two", "2"}, {"three", "3"}, {"four", "4"}, {"five", "5"}, {"six", "6"},
					{"seven", "7"}, {"eight", "8"}, {"nine", "9"}, {"ten", "10"}, {"eleven", "11"},
					{"twelve", "12"}, {"thirty-six", "36"},
				},
				expectedError: nil,
			},
			{
				// 4. Get the union between 3 sorted sets with scores.
				// Use MAX aggregate.
				name: "4. Get the union between 3 sorted sets with scores.",
				presetValues: map[string]interface{}{
					"ZunionKey9": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 100}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZunionKey10": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 80},
					}),
					"ZunionKey11": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1000}, {Value: "eight", Score: 800},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12}, {Value: "thirty-six", Score: 72},
					}),
				},
				command: []string{"ZUNION", "ZunionKey9", "ZunionKey10", "ZunionKey11", "WITHSCORES", "AGGREGATE", "MAX"},
				expectedResponse: [][]string{
					{"one", "1000"}, {"two", "2"}, {"three", "3"}, {"four", "4"}, {"five", "5"}, {"six", "6"},
					{"seven", "7"}, {"eight", "800"}, {"nine", "9"}, {"ten", "10"}, {"eleven", "11"},
					{"twelve", "12"}, {"thirty-six", "72"},
				},
				expectedError: nil,
			},
			{
				// 5. Get the union between 3 sorted sets with scores.
				// Use SUM aggregate with weights modifier.
				name: "5. Get the union between 3 sorted sets with scores.",
				presetValues: map[string]interface{}{
					"ZunionKey12": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 100}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZunionKey13": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 80},
					}),
					"ZunionKey14": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1000}, {Value: "eight", Score: 800},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				command: []string{"ZUNION", "ZunionKey12", "ZunionKey13", "ZunionKey14", "WITHSCORES", "AGGREGATE", "SUM", "WEIGHTS", "1", "2", "3"},
				expectedResponse: [][]string{
					{"one", "3102"}, {"two", "6"}, {"three", "3"}, {"four", "4"}, {"five", "5"}, {"six", "6"},
					{"seven", "7"}, {"eight", "2568"}, {"nine", "27"}, {"ten", "30"}, {"eleven", "22"},
					{"twelve", "60"}, {"thirty-six", "72"},
				},
				expectedError: nil,
			},
			{
				// 6. Get the union between 3 sorted sets with scores.
				// Use MAX aggregate with added weights.
				name: "6. Get the union between 3 sorted sets with scores.",
				presetValues: map[string]interface{}{
					"ZunionKey15": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 100}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZunionKey16": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 80},
					}),
					"ZunionKey17": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1000}, {Value: "eight", Score: 800},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				command: []string{"ZUNION", "ZunionKey15", "ZunionKey16", "ZunionKey17", "WITHSCORES", "AGGREGATE", "MAX", "WEIGHTS", "1", "2", "3"},
				expectedResponse: [][]string{
					{"one", "3000"}, {"two", "4"}, {"three", "3"}, {"four", "4"}, {"five", "5"}, {"six", "6"},
					{"seven", "7"}, {"eight", "2400"}, {"nine", "27"}, {"ten", "30"}, {"eleven", "22"},
					{"twelve", "36"}, {"thirty-six", "72"},
				},
				expectedError: nil,
			},
			{
				// 7. Get the union between 3 sorted sets with scores.
				// Use MIN aggregate with added weights.
				name: "7. Get the union between 3 sorted sets with scores.",
				presetValues: map[string]interface{}{
					"ZunionKey18": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 100}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZunionKey19": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 80},
					}),
					"ZunionKey20": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1000}, {Value: "eight", Score: 800},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				command: []string{"ZUNION", "ZunionKey18", "ZunionKey19", "ZunionKey20", "WITHSCORES", "AGGREGATE", "MIN", "WEIGHTS", "1", "2", "3"},
				expectedResponse: [][]string{
					{"one", "2"}, {"two", "2"}, {"three", "3"}, {"four", "4"}, {"five", "5"}, {"six", "6"}, {"seven", "7"},
					{"eight", "8"}, {"nine", "27"}, {"ten", "30"}, {"eleven", "22"}, {"twelve", "24"}, {"thirty-six", "72"},
				},
				expectedError: nil,
			},
			{
				name: "8. Throw an error if there are more weights than keys",
				presetValues: map[string]interface{}{
					"ZunionKey21": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZunionKey22": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
				},
				command:          []string{"ZUNION", "ZunionKey21", "ZunionKey22", "WEIGHTS", "1", "2", "3"},
				expectedResponse: nil,
				expectedError:    errors.New("number of weights should match number of keys"),
			},
			{
				name: "9. Throw an error if there are fewer weights than keys",
				presetValues: map[string]interface{}{
					"ZunionKey23": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZunionKey24": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
					}),
					"ZunionKey25": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
				},
				command:          []string{"ZUNION", "ZunionKey23", "ZunionKey24", "ZunionKey25", "WEIGHTS", "5", "4"},
				expectedResponse: nil,
				expectedError:    errors.New("number of weights should match number of keys"),
			},
			{
				name: "10. Throw an error if there are no keys provided",
				presetValues: map[string]interface{}{
					"ZunionKey26": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
					"ZunionKey27": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
					"ZunionKey28": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
				},
				command:          []string{"ZUNION", "WEIGHTS", "5", "4"},
				expectedResponse: nil,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name: "11. Throw an error if any of the provided keys are not sorted sets",
				presetValues: map[string]interface{}{
					"ZunionKey29": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZunionKey30": "Default value",
					"ZunionKey31": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
				},
				command:          []string{"ZUNION", "ZunionKey29", "ZunionKey30", "ZunionKey31"},
				expectedResponse: nil,
				expectedError:    errors.New("value at ZunionKey30 is not a sorted set"),
			},
			{
				name: "12. If any of the keys does not exist, skip it.",
				presetValues: map[string]interface{}{
					"ZunionKey32": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11},
					}),
					"ZunionKey33": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				command: []string{"ZUNION", "non-existent", "ZunionKey32", "ZunionKey33"},
				expectedResponse: [][]string{
					{"one"}, {"two"}, {"thirty-six"}, {"twelve"}, {"eleven"},
					{"seven"}, {"eight"}, {"nine"}, {"ten"},
				},
				expectedError: nil,
			},
			{
				name:          "13. Command too short",
				command:       []string{"ZUNION"},
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
						case *sorted_set.SortedSet:
							command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(key)}
							for _, member := range value.(*sorted_set.SortedSet).GetAll() {
								command = append(command, []resp.Value{
									resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
									resp.StringValue(string(member.Value)),
								}...)
							}
							expected = strconv.Itoa(value.(*sorted_set.SortedSet).Cardinality())
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
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), res.Error().Error())
					}
					return
				}

				if len(res.Array()) != len(test.expectedResponse) {
					t.Errorf("expected response array of length %d, got %d", len(test.expectedResponse), len(res.Array()))
				}

				for _, item := range res.Array() {
					value := item.Array()[0].String()
					score := func() string {
						if len(item.Array()) == 2 {
							return item.Array()[1].String()
						}
						return ""
					}()
					if !slices.ContainsFunc(test.expectedResponse, func(expected []string) bool {
						return expected[0] == value
					}) {
						t.Errorf("unexpected member \"%s\" in response", value)
					}
					if score != "" {
						for _, expected := range test.expectedResponse {
							if expected[0] == value && expected[1] != score {
								t.Errorf("expected score for member \"%s\" to be %s, got %s", value, expected[1], score)
							}
						}
					}
				}
			})
		}
	})

	t.Run("Test_HandleZUNIONSTORE", func(t *testing.T) {
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
			presetValues     map[string]interface{}
			destination      string
			command          []string
			expectedValue    *sorted_set.SortedSet
			expectedResponse int
			expectedError    error
		}{
			{
				name:   "1. Get the union between 2 sorted sets.",
				preset: true,
				presetValues: map[string]interface{}{
					"ZunionStoreKey1": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5},
					}),
					"ZunionStoreKey2": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
				},
				destination: "ZunionStoreDestinationKey1",
				command:     []string{"ZUNIONSTORE", "ZunionStoreDestinationKey1", "ZunionStoreKey1", "ZunionStoreKey2"},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 1}, {Value: "two", Score: 2},
					{Value: "three", Score: 6}, {Value: "four", Score: 8},
					{Value: "five", Score: 10}, {Value: "six", Score: 6},
					{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
				}),
				expectedResponse: 8,
				expectedError:    nil,
			},
			{
				// 2. Get the union between 3 sorted sets with scores.
				// By default, the SUM aggregate will be used.
				name:   "2. Get the union between 3 sorted sets with scores.",
				preset: true,
				presetValues: map[string]interface{}{
					"ZunionStoreKey3": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZunionStoreKey4": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 8},
					}),
					"ZunionStoreKey5": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "eight", Score: 8},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12}, {Value: "thirty-six", Score: 36},
					}),
				},
				destination: "ZunionStoreDestinationKey2",
				command:     []string{"ZUNIONSTORE", "ZunionStoreDestinationKey2", "ZunionStoreKey3", "ZunionStoreKey4", "ZunionStoreKey5", "WITHSCORES"},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 3}, {Value: "two", Score: 4}, {Value: "three", Score: 3}, {Value: "four", Score: 4},
					{Value: "five", Score: 5}, {Value: "six", Score: 6}, {Value: "seven", Score: 7}, {Value: "eight", Score: 24},
					{Value: "nine", Score: 9}, {Value: "ten", Score: 10}, {Value: "eleven", Score: 11},
					{Value: "twelve", Score: 24}, {Value: "thirty-six", Score: 72},
				}),
				expectedResponse: 13,
				expectedError:    nil,
			},
			{
				// 3. Get the union between 3 sorted sets with scores.
				// Use MIN aggregate.
				name:   "3. Get the union between 3 sorted sets with scores.",
				preset: true,
				presetValues: map[string]interface{}{
					"ZunionStoreKey6": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 100}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZunionStoreKey7": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 80},
					}),
					"ZunionStoreKey8": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1000}, {Value: "eight", Score: 800},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12}, {Value: "thirty-six", Score: 72},
					}),
				},
				destination: "ZunionStoreDestinationKey3",
				command:     []string{"ZUNIONSTORE", "ZunionStoreDestinationKey3", "ZunionStoreKey6", "ZunionStoreKey7", "ZunionStoreKey8", "WITHSCORES", "AGGREGATE", "MIN"},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 1}, {Value: "two", Score: 2}, {Value: "three", Score: 3}, {Value: "four", Score: 4},
					{Value: "five", Score: 5}, {Value: "six", Score: 6}, {Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					{Value: "nine", Score: 9}, {Value: "ten", Score: 10}, {Value: "eleven", Score: 11},
					{Value: "twelve", Score: 12}, {Value: "thirty-six", Score: 36},
				}),
				expectedResponse: 13,
				expectedError:    nil,
			},
			{
				// 4. Get the union between 3 sorted sets with scores.
				// Use MAX aggregate.
				name:   "4. Get the union between 3 sorted sets with scores.",
				preset: true,
				presetValues: map[string]interface{}{
					"ZunionStoreKey9": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 100}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZunionStoreKey10": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 80},
					}),
					"ZunionStoreKey11": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1000}, {Value: "eight", Score: 800},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12}, {Value: "thirty-six", Score: 72},
					}),
				},
				destination: "ZunionStoreDestinationKey4",
				command: []string{
					"ZUNIONSTORE", "ZunionStoreDestinationKey4", "ZunionStoreKey9", "ZunionStoreKey10", "ZunionStoreKey11", "WITHSCORES", "AGGREGATE", "MAX",
				},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 1000}, {Value: "two", Score: 2}, {Value: "three", Score: 3}, {Value: "four", Score: 4},
					{Value: "five", Score: 5}, {Value: "six", Score: 6}, {Value: "seven", Score: 7}, {Value: "eight", Score: 800},
					{Value: "nine", Score: 9}, {Value: "ten", Score: 10}, {Value: "eleven", Score: 11},
					{Value: "twelve", Score: 12}, {Value: "thirty-six", Score: 72},
				}),
				expectedResponse: 13,
				expectedError:    nil,
			},
			{
				// 5. Get the union between 3 sorted sets with scores.
				// Use SUM aggregate with weights modifier.
				name:   "5. Get the union between 3 sorted sets with scores.",
				preset: true,
				presetValues: map[string]interface{}{
					"ZunionStoreKey12": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 100}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZunionStoreKey13": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 80},
					}),
					"ZunionStoreKey14": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1000}, {Value: "eight", Score: 800},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				destination: "ZunionStoreDestinationKey5",
				command: []string{
					"ZUNIONSTORE", "ZunionStoreDestinationKey5", "ZunionStoreKey12", "ZunionStoreKey13", "ZunionStoreKey14",
					"WITHSCORES", "AGGREGATE", "SUM", "WEIGHTS", "1", "2", "3",
				},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 3102}, {Value: "two", Score: 6}, {Value: "three", Score: 3}, {Value: "four", Score: 4},
					{Value: "five", Score: 5}, {Value: "six", Score: 6}, {Value: "seven", Score: 7}, {Value: "eight", Score: 2568},
					{Value: "nine", Score: 27}, {Value: "ten", Score: 30}, {Value: "eleven", Score: 22},
					{Value: "twelve", Score: 60}, {Value: "thirty-six", Score: 72},
				}),
				expectedResponse: 13,
				expectedError:    nil,
			},
			{
				// 6. Get the union between 3 sorted sets with scores.
				// Use MAX aggregate with added weights.
				name:   "6. Get the union between 3 sorted sets with scores.",
				preset: true,
				presetValues: map[string]interface{}{
					"ZunionStoreKey15": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 100}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZunionStoreKey16": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 80},
					}),
					"ZunionStoreKey17": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1000}, {Value: "eight", Score: 800},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				destination: "ZunionStoreDestinationKey6",
				command: []string{
					"ZUNIONSTORE", "ZunionStoreDestinationKey6", "ZunionStoreKey15", "ZunionStoreKey16", "ZunionStoreKey17",
					"WITHSCORES", "AGGREGATE", "MAX", "WEIGHTS", "1", "2", "3"},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 3000}, {Value: "two", Score: 4}, {Value: "three", Score: 3}, {Value: "four", Score: 4},
					{Value: "five", Score: 5}, {Value: "six", Score: 6}, {Value: "seven", Score: 7}, {Value: "eight", Score: 2400},
					{Value: "nine", Score: 27}, {Value: "ten", Score: 30}, {Value: "eleven", Score: 22},
					{Value: "twelve", Score: 36}, {Value: "thirty-six", Score: 72},
				}),
				expectedResponse: 13,
				expectedError:    nil,
			},
			{
				// 7. Get the union between 3 sorted sets with scores.
				// Use MIN aggregate with added weights.
				name:   "7. Get the union between 3 sorted sets with scores.",
				preset: true,
				presetValues: map[string]interface{}{
					"ZunionStoreKey18": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 100}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZunionStoreKey19": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11}, {Value: "eight", Score: 80},
					}),
					"ZunionStoreKey20": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1000}, {Value: "eight", Score: 800},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				destination: "ZunionStoreDestinationKey7",
				command: []string{
					"ZUNIONSTORE", "ZunionStoreDestinationKey7", "ZunionStoreKey18", "ZunionStoreKey19", "ZunionStoreKey20",
					"WITHSCORES", "AGGREGATE", "MIN", "WEIGHTS", "1", "2", "3",
				},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 2}, {Value: "two", Score: 2}, {Value: "three", Score: 3}, {Value: "four", Score: 4},
					{Value: "five", Score: 5}, {Value: "six", Score: 6}, {Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					{Value: "nine", Score: 27}, {Value: "ten", Score: 30}, {Value: "eleven", Score: 22},
					{Value: "twelve", Score: 24}, {Value: "thirty-six", Score: 72},
				}),
				expectedResponse: 13,
				expectedError:    nil,
			},
			{
				name:   "8. Throw an error if there are more weights than keys",
				preset: true,
				presetValues: map[string]interface{}{
					"ZunionStoreKey21": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZunionStoreKey22": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
				},
				destination:      "ZunionStoreDestinationKey8",
				command:          []string{"ZUNIONSTORE", "ZunionStoreDestinationKey8", "ZunionStoreKey21", "ZunionStoreKey22", "WEIGHTS", "1", "2", "3"},
				expectedResponse: 0,
				expectedError:    errors.New("number of weights should match number of keys"),
			},
			{
				name:   "9. Throw an error if there are fewer weights than keys",
				preset: true,
				presetValues: map[string]interface{}{
					"ZunionStoreKey23": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZunionStoreKey24": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
					}),
					"ZunionStoreKey25": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
				},
				destination:      "ZunionStoreDestinationKey9",
				command:          []string{"ZUNIONSTORE", "ZunionStoreDestinationKey9", "ZunionStoreKey23", "ZunionStoreKey24", "ZunionStoreKey25", "WEIGHTS", "5", "4"},
				expectedResponse: 0,
				expectedError:    errors.New("number of weights should match number of keys"),
			},
			{
				name:   "10. Throw an error if there are no keys provided",
				preset: true,
				presetValues: map[string]interface{}{
					"ZunionStoreKey26": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
					"ZunionStoreKey27": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
					"ZunionStoreKey28": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
				},
				command:          []string{"ZUNIONSTORE", "WEIGHTS", "5", "4"},
				expectedResponse: 0,
				expectedError:    errors.New(constants.WrongArgsResponse),
			},
			{
				name:   "11. Throw an error if any of the provided keys are not sorted sets",
				preset: true,
				presetValues: map[string]interface{}{
					"ZunionStoreKey29": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "three", Score: 3}, {Value: "four", Score: 4},
						{Value: "five", Score: 5}, {Value: "six", Score: 6},
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					}),
					"ZunionStoreKey30": "Default value",
					"ZunionStoreKey31": sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: "one", Score: 1}}),
				},
				destination:      "ZunionStoreDestinationKey11",
				command:          []string{"ZUNIONSTORE", "ZunionStoreDestinationKey11", "ZunionStoreKey29", "ZunionStoreKey30", "ZunionStoreKey31"},
				expectedResponse: 0,
				expectedError:    errors.New("value at ZunionStoreKey30 is not a sorted set"),
			},
			{
				name:   "12. If any of the keys does not exist, skip it.",
				preset: true,
				presetValues: map[string]interface{}{
					"ZunionStoreKey32": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "one", Score: 1}, {Value: "two", Score: 2},
						{Value: "thirty-six", Score: 36}, {Value: "twelve", Score: 12},
						{Value: "eleven", Score: 11},
					}),
					"ZunionStoreKey33": sorted_set.NewSortedSet([]sorted_set.MemberParam{
						{Value: "seven", Score: 7}, {Value: "eight", Score: 8},
						{Value: "nine", Score: 9}, {Value: "ten", Score: 10},
						{Value: "twelve", Score: 12},
					}),
				},
				destination: "ZunionStoreDestinationKey12",
				command:     []string{"ZUNIONSTORE", "ZunionStoreDestinationKey12", "non-existent", "ZunionStoreKey32", "ZunionStoreKey33"},
				expectedValue: sorted_set.NewSortedSet([]sorted_set.MemberParam{
					{Value: "one", Score: 1}, {Value: "two", Score: 2}, {Value: "seven", Score: 7}, {Value: "eight", Score: 8},
					{Value: "nine", Score: 9}, {Value: "ten", Score: 10}, {Value: "eleven", Score: 11}, {Value: "twelve", Score: 24},
					{Value: "thirty-six", Score: 36},
				}),
				expectedResponse: 9,
				expectedError:    nil,
			},
			{
				name:             "13. Command too short",
				preset:           false,
				command:          []string{"ZUNIONSTORE"},
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
						case *sorted_set.SortedSet:
							command = []resp.Value{resp.StringValue("ZADD"), resp.StringValue(key)}
							for _, member := range value.(*sorted_set.SortedSet).GetAll() {
								command = append(command, []resp.Value{
									resp.StringValue(strconv.FormatFloat(float64(member.Score), 'f', -1, 64)),
									resp.StringValue(string(member.Value)),
								}...)
							}
							expected = strconv.Itoa(value.(*sorted_set.SortedSet).Cardinality())
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
						t.Errorf("expected error \"%s\", got \"%s\"", test.expectedError.Error(), res.Error().Error())
					}
					return
				}

				if res.Integer() != test.expectedResponse {
					t.Errorf("expected response %d, got %d", test.expectedResponse, res.Integer())
				}

				// Check if the resulting sorted set has the expected members/scores
				if test.expectedValue == nil {
					return
				}

				if err = client.WriteArray([]resp.Value{
					resp.StringValue("ZRANGE"),
					resp.StringValue(test.destination),
					resp.StringValue("-inf"),
					resp.StringValue("+inf"),
					resp.StringValue("BYSCORE"),
					resp.StringValue("WITHSCORES"),
				}); err != nil {
					t.Error(err)
				}

				res, _, err = client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				if len(res.Array()) != test.expectedValue.Cardinality() {
					t.Errorf("expected resulting set %s to have cardinality %d, got %d",
						test.destination, test.expectedValue.Cardinality(), len(res.Array()))
				}

				for _, member := range res.Array() {
					value := sorted_set.Value(member.Array()[0].String())
					score := sorted_set.Score(member.Array()[1].Float())
					if !test.expectedValue.Contains(value) {
						t.Errorf("unexpected value %s in resulting sorted set", value)
					}
					if test.expectedValue.Get(value).Score != score {
						t.Errorf("expected value %s to have score %v, got %v", value, test.expectedValue.Get(value).Score, score)
					}
				}
			})
		}
	})
}
