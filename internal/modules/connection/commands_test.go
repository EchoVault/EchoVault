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

package connection_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/echovault/echovault/internal/modules/connection"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/echovault/echovault/echovault"
	"github.com/echovault/echovault/internal"
	"github.com/echovault/echovault/internal/config"
	"github.com/echovault/echovault/internal/constants"
	"github.com/tidwall/resp"
)

func setUpServer(port int, requirePass bool, aclConfig string) (*echovault.EchoVault, error) {
	conf := config.Config{
		BindAddr:       "localhost",
		Port:           uint16(port),
		DataDir:        "",
		EvictionPolicy: constants.NoEviction,
		RequirePass:    requirePass,
		Password:       "password1",
		AclConfig:      aclConfig,
	}

	mockServer, err := echovault.NewEchoVault(
		echovault.WithConfig(conf),
	)
	if err != nil {
		return nil, err
	}

	// Add the initial test users to the ACL module.
	for _, user := range generateInitialTestUsers() {
		if _, err := mockServer.ACLSetUser(user); err != nil {
			return nil, err
		}
	}

	return mockServer, nil
}

func generateInitialTestUsers() []echovault.User {
	return []echovault.User{
		{
			// User with both hash password and plaintext password.
			Username:          "with_password_user",
			Enabled:           true,
			IncludeCategories: []string{"*"},
			IncludeCommands:   []string{"*"},
			AddPlainPasswords: []string{"password2"},
			AddHashPasswords:  []string{generateSHA256Password("password3")},
		},
		{
			// User with NoPassword option.
			Username:          "no_password_user",
			Enabled:           true,
			NoPassword:        true,
			AddPlainPasswords: []string{"password4"},
		},
		{
			// Disabled user.
			Username:          "disabled_user",
			Enabled:           false,
			AddPlainPasswords: []string{"password5"},
		},
	}
}

func generateSHA256Password(plain string) string {
	h := sha256.New()
	h.Write([]byte(plain))
	return hex.EncodeToString(h.Sum(nil))
}

func Test_Connection(t *testing.T) {
	port, err := internal.GetFreePort()
	if err != nil {
		t.Error(err)
		return
	}

	mockServer, err := setUpServer(port, true, "")
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

	t.Run("Test_HandleAuth", func(t *testing.T) {
		t.Parallel()

		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			if conn != nil {
				_ = conn.Close()
			}
		}()

		r := resp.NewConn(conn)

		tests := []struct {
			name    string
			cmd     []resp.Value
			wantRes string
			wantErr string
		}{
			{
				name:    "1. Authenticate with default user without specifying username",
				cmd:     []resp.Value{resp.StringValue("AUTH"), resp.StringValue("password1")},
				wantRes: "OK",
				wantErr: "",
			},
			{
				name: "2. Authenticate with plaintext password",
				cmd: []resp.Value{
					resp.StringValue("AUTH"),
					resp.StringValue("with_password_user"),
					resp.StringValue("password2"),
				},
				wantRes: "OK",
				wantErr: "",
			},
			{
				name: "3. Authenticate with SHA256 password",
				cmd: []resp.Value{
					resp.StringValue("AUTH"),
					resp.StringValue("with_password_user"),
					resp.StringValue("password3"),
				},
				wantRes: "OK",
				wantErr: "",
			},
			{
				name: "4. Authenticate with no password user",
				cmd: []resp.Value{
					resp.StringValue("AUTH"),
					resp.StringValue("no_password_user"),
					resp.StringValue("password4"),
				},
				wantRes: "OK",
				wantErr: "",
			},
			{
				name: "5. Fail to authenticate with disabled user",
				cmd: []resp.Value{
					resp.StringValue("AUTH"),
					resp.StringValue("disabled_user"),
					resp.StringValue("password5"),
				},
				wantRes: "",
				wantErr: "Error user disabled_user is disabled",
			},
			{
				name: "6. Fail to authenticate with non-existent user",
				cmd: []resp.Value{
					resp.StringValue("AUTH"),
					resp.StringValue("non_existent_user"),
					resp.StringValue("password6"),
				},
				wantRes: "",
				wantErr: "Error no user with username non_existent_user",
			},
			{
				name: "7. Fail to authenticate with the wrong password",
				cmd: []resp.Value{
					resp.StringValue("AUTH"),
					resp.StringValue("with_password_user"),
					resp.StringValue("wrong_password"),
				},
				wantRes: "",
				wantErr: "Error could not authenticate user",
			},
			{
				name:    "8. Command too short",
				cmd:     []resp.Value{resp.StringValue("AUTH")},
				wantRes: "",
				wantErr: fmt.Sprintf("Error %s", constants.WrongArgsResponse),
			},
			{
				name: "9. Command too long",
				cmd: []resp.Value{
					resp.StringValue("AUTH"),
					resp.StringValue("user"),
					resp.StringValue("password1"),
					resp.StringValue("password2"),
				},
				wantRes: "",
				wantErr: fmt.Sprintf("Error %s", constants.WrongArgsResponse),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if err = r.WriteArray(test.cmd); err != nil {
					t.Error(err)
				}
				rv, _, err := r.ReadValue()
				if err != nil {
					t.Error(err)
				}
				if test.wantErr != "" {
					if rv.Error().Error() != test.wantErr {
						t.Errorf("expected error response \"%s\", got \"%s\"", test.wantErr, rv.Error().Error())
					}
					return
				}
				if rv.String() != test.wantRes {
					t.Errorf("expected response \"%s\", got \"%s\"", test.wantRes, rv.String())
				}
			})
		}
	})

	t.Run("Test_HandlePing", func(t *testing.T) {
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
			command     []resp.Value
			expected    string
			expectedErr error
		}{
			{
				command:     []resp.Value{resp.StringValue("PING")},
				expected:    "PONG",
				expectedErr: nil,
			},
			{
				command:     []resp.Value{resp.StringValue("PING"), resp.StringValue("Hello, world!")},
				expected:    "Hello, world!",
				expectedErr: nil,
			},
			{
				command: []resp.Value{
					resp.StringValue("PING"),
					resp.StringValue("Hello, world!"),
					resp.StringValue("Once more"),
				},
				expected:    "",
				expectedErr: errors.New(constants.WrongArgsResponse),
			},
		}

		for _, test := range tests {
			if err = client.WriteArray(test.command); err != nil {
				t.Error(err)
				return
			}

			res, _, err := client.ReadValue()
			if err != nil {
				t.Error(err)
			}

			if test.expectedErr != nil {
				if !strings.Contains(res.Error().Error(), test.expectedErr.Error()) {
					t.Errorf("expected error \"%s\", got \"%s\"", test.expectedErr.Error(), res.Error().Error())
				}
				continue
			}

			if res.String() != test.expected {
				t.Errorf("expected response \"%s\", got \"%s\"", test.expected, res.String())
			}
		}
	})

	t.Run("Test_HandleEcho", func(t *testing.T) {
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
			command     []resp.Value
			expected    string
			expectedErr error
		}{
			{
				command:     []resp.Value{resp.StringValue("ECHO"), resp.StringValue("Hello, EchoVault!")},
				expected:    "Hello, EchoVault!",
				expectedErr: nil,
			},
			{
				command:     []resp.Value{resp.StringValue("ECHO")},
				expected:    "",
				expectedErr: errors.New(constants.WrongArgsResponse),
			},
			{
				command: []resp.Value{
					resp.StringValue("ECHO"),
					resp.StringValue("Hello, EchoVault!"),
					resp.StringValue("Once more"),
				},
				expected:    "",
				expectedErr: errors.New(constants.WrongArgsResponse),
			},
		}

		for _, test := range tests {
			if err = client.WriteArray(test.command); err != nil {
				t.Error(err)
				return
			}

			res, _, err := client.ReadValue()
			if err != nil {
				t.Error(err)
			}

			if test.expectedErr != nil {
				if !strings.Contains(res.Error().Error(), test.expectedErr.Error()) {
					t.Errorf("expected error \"%s\", got \"%s\"", test.expectedErr.Error(), res.Error().Error())
				}
				continue
			}

			if res.String() != test.expected {
				t.Errorf("expected response \"%s\", got \"%s\"", test.expected, res.String())
			}
		}
	})

	t.Run("Test_HandleHello", func(t *testing.T) {
		t.Parallel()

		port, err := internal.GetFreePort()
		if err != nil {
			t.Error(err)
			return
		}
		mockServer, err := setUpServer(port, true, "")
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

		tests := []struct {
			name    string
			command []resp.Value
			wantRes []byte
		}{
			{
				name:    "1. Hello",
				command: []resp.Value{resp.StringValue("HELLO")},
				wantRes: connection.BuildHelloResponse(
					internal.ServerInfo{
						Server:  "echovault",
						Version: constants.Version,
						Id:      "",
						Mode:    "standalone",
						Role:    "master",
						Modules: mockServer.ListModules(),
					},
					internal.ConnectionInfo{
						Id:       1,
						Name:     "",
						Protocol: 2,
						Database: 0,
					},
				),
			},
			{
				name:    "2. Hello 2",
				command: []resp.Value{resp.StringValue("HELLO"), resp.StringValue("2")},
				wantRes: connection.BuildHelloResponse(
					internal.ServerInfo{
						Server:  "echovault",
						Version: constants.Version,
						Id:      "",
						Mode:    "standalone",
						Role:    "master",
						Modules: mockServer.ListModules(),
					},
					internal.ConnectionInfo{
						Id:       2,
						Name:     "",
						Protocol: 2,
						Database: 0,
					},
				),
			},
			{
				name:    "3. Hello 3",
				command: []resp.Value{resp.StringValue("HELLO"), resp.StringValue("3")},
				wantRes: connection.BuildHelloResponse(
					internal.ServerInfo{
						Server:  "echovault",
						Version: constants.Version,
						Id:      "",
						Mode:    "standalone",
						Role:    "master",
						Modules: mockServer.ListModules(),
					},
					internal.ConnectionInfo{
						Id:       3,
						Name:     "",
						Protocol: 3,
						Database: 0,
					},
				),
			},
			{
				name: "4. Hello with auth success",
				command: []resp.Value{
					resp.StringValue("HELLO"),
					resp.StringValue("3"),
					resp.StringValue("AUTH"),
					resp.StringValue("default"),
					resp.StringValue("password1"),
				},
				wantRes: connection.BuildHelloResponse(
					internal.ServerInfo{
						Server:  "echovault",
						Version: constants.Version,
						Id:      "",
						Mode:    "standalone",
						Role:    "master",
						Modules: mockServer.ListModules(),
					},
					internal.ConnectionInfo{
						Id:       4,
						Name:     "",
						Protocol: 3,
						Database: 0,
					},
				),
			},
			{
				name: "5. Hello with auth failure",
				command: []resp.Value{
					resp.StringValue("HELLO"),
					resp.StringValue("3"),
					resp.StringValue("AUTH"),
					resp.StringValue("default"),
					resp.StringValue("password2"),
				},
				wantRes: []byte("-Error could not authenticate user\r\n"),
			},
			{
				name: "6. Hello with auth and set client name",
				command: []resp.Value{
					resp.StringValue("HELLO"),
					resp.StringValue("3"),
					resp.StringValue("AUTH"),
					resp.StringValue("default"),
					resp.StringValue("password1"),
					resp.StringValue("SETNAME"),
					resp.StringValue("client6"),
				},
				wantRes: connection.BuildHelloResponse(
					internal.ServerInfo{
						Server:  "echovault",
						Version: constants.Version,
						Id:      "",
						Mode:    "standalone",
						Role:    "master",
						Modules: mockServer.ListModules(),
					},
					internal.ConnectionInfo{
						Id:       6,
						Name:     "",
						Protocol: 3,
						Database: 0,
					},
				),
			},
			{
				name: "7. Command too long",
				command: []resp.Value{
					resp.StringValue("HELLO"),
					resp.StringValue("3"),
					resp.StringValue("AUTH"),
					resp.StringValue("default"),
					resp.StringValue("password1"),
					resp.StringValue("SETNAME"),
					resp.StringValue("client6"),
					resp.StringValue("extra_arg"),
				},
				wantRes: []byte(fmt.Sprintf("-Error %s\r\n", constants.WrongArgsResponse)),
			},
		}

		for i := 0; i < len(tests); i++ {
			conn, err := internal.GetConnection("localhost", port)
			if err != nil {
				t.Error(err)
				return
			}
			client := resp.NewConn(conn)

			if err = client.WriteArray(tests[i].command); err != nil {
				t.Error(err)
				return
			}

			buf := bufio.NewReader(conn)
			res, err := internal.ReadMessage(buf)
			if err != nil {
				t.Error(err)
				return
			}

			if !bytes.Equal(tests[i].wantRes, res) {
				t.Errorf("expected byte resposne:\n%s, \n\ngot:\n%s", string(tests[i].wantRes), string(res))
				return
			}

			// Close connection
			_ = conn.Close()
		}
	})

	t.Run("Test_HandleSelect", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name       string
			database   int
			wantDBErr  error
			setCommand []resp.Value
			getCommand []resp.Value
			getWantRes []resp.Value
		}{
			{
				name:      "1. Default database 0",
				database:  0,
				wantDBErr: nil,
				setCommand: []resp.Value{
					resp.StringValue("MSET"),
					resp.StringValue("key1"), resp.StringValue("value-01"),
					resp.StringValue("key2"), resp.StringValue("value-02"),
					resp.StringValue("key3"), resp.StringValue("value-03"),
				},
				getCommand: []resp.Value{
					resp.StringValue("MGET"),
					resp.StringValue("key1"),
					resp.StringValue("key2"),
					resp.StringValue("key3"),
				},
				getWantRes: []resp.Value{
					resp.StringValue("value-01"),
					resp.StringValue("value-02"),
					resp.StringValue("value-03"),
				},
			},
			{
				name:      "2. Select database 1",
				database:  1,
				wantDBErr: nil,
				setCommand: []resp.Value{
					resp.StringValue("MSET"),
					resp.StringValue("key1"), resp.StringValue("value-11"),
					resp.StringValue("key2"), resp.StringValue("value-12"),
					resp.StringValue("key3"), resp.StringValue("value-13"),
				},
				getCommand: []resp.Value{
					resp.StringValue("MGET"),
					resp.StringValue("key1"),
					resp.StringValue("key2"),
					resp.StringValue("key3"),
				},
				getWantRes: []resp.Value{
					resp.StringValue("value-11"),
					resp.StringValue("value-12"),
					resp.StringValue("value-13"),
				},
			},
			{
				name:      "3. Error when selecting database < 0",
				database:  -1,
				wantDBErr: errors.New("database must be >= 0"),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				conn, err := internal.GetConnection("localhost", port)
				if err != nil {
					t.Error(err)
					return
				}
				client := resp.NewConn(conn)

				// Authenticate the connection
				if err = client.WriteArray([]resp.Value{
					resp.StringValue("AUTH"),
					resp.StringValue("password1"),
				}); err != nil {
					t.Error(err)
					return
				}

				res, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
					return
				}
				if !strings.EqualFold(res.String(), "ok") {
					t.Errorf("expected OK auth response, got \"%s\"", res.String())
					return
				}

				// If database is not 0, execute the select command
				if test.database != 0 {
					if err = client.WriteArray([]resp.Value{
						resp.StringValue("SELECT"),
						resp.StringValue(strconv.Itoa(test.database)),
					}); err != nil {
						t.Error(err)
						return
					}
					res, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
						return
					}
					if test.wantDBErr != nil {
						// If we expect a select error, check that it's the expected error.
						if !strings.Contains(res.Error().Error(), test.wantDBErr.Error()) {
							t.Errorf("expected error response to contain \"%s\", \"%s\"", test.wantDBErr.Error(), res.Error().Error())
							return
						}
						return
					} else {
						// We do not expect an error, check if it's an OK response.
						if !strings.EqualFold(res.String(), "ok") {
							t.Errorf("expected OK response, got \"%s\"", res.String())
							return
						}
					}
				}

				// Execute command to set values
				if err = client.WriteArray(test.setCommand); err != nil {
					t.Error(err)
					return
				}
				res, _, err = client.ReadValue()
				if err != nil {
					t.Error(err)
					return
				}
				if !strings.EqualFold(res.String(), "ok") {
					t.Errorf("expected OK set response, got \"%s\"", res.String())
					return
				}

				// Execute commands to get values.
				if err = client.WriteArray(test.getCommand); err != nil {
					t.Error(err)
					return
				}
				res, _, err = client.ReadValue()
				if err != nil {
					t.Error(err)
					return
				}
				if !reflect.DeepEqual(res.Array(), test.getWantRes) {
					t.Errorf("expected response %+v, got %+v", test.getWantRes, res.Array())
					return
				}
			})
		}
	})

	t.Run("Test_HandleSwapDBs", func(t *testing.T) {
		t.Parallel()

		port, err := internal.GetFreePort()
		if err != nil {
			t.Error(err)
			return
		}
		mockServer, err := setUpServer(port, false, "")
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

		tests := []struct {
			name         string
			presetValues map[int]map[string]string
			database0    string
			database1    string
			getCommand   []resp.Value
			swapCommand  []resp.Value
			want0        []resp.Value
			want1        []resp.Value
			wantErr      error
		}{
			{
				name: "1. Successfully swap databases",
				presetValues: map[int]map[string]string{
					0: {"key1": "value-01", "key2": "value-02", "key3": "value-03", "key4": "value-04", "key5": "value-05"},
					1: {"key1": "value-11", "key2": "value-12", "key3": "value-13", "key4": "value-14", "key5": "value-15"},
				},
				database0: "0",
				database1: "1",
				getCommand: []resp.Value{
					resp.StringValue("MGET"),
					resp.StringValue("key1"), resp.StringValue("key2"), resp.StringValue("key3"),
					resp.StringValue("key4"), resp.StringValue("key5"),
				},
				swapCommand: []resp.Value{
					resp.StringValue("SWAPDB"), resp.StringValue("0"), resp.StringValue("1"),
				},
				want0: []resp.Value{
					resp.StringValue("value-01"), resp.StringValue("value-02"), resp.StringValue("value-03"),
					resp.StringValue("value-04"), resp.StringValue("value-05"),
				},
				want1: []resp.Value{
					resp.StringValue("value-11"), resp.StringValue("value-12"), resp.StringValue("value-13"),
					resp.StringValue("value-14"), resp.StringValue("value-15"),
				},
				wantErr: nil,
			},
			{
				name:         "2. First database index is not an integer",
				presetValues: nil,
				database0:    "index0",
				database1:    "1",
				getCommand:   make([]resp.Value, 0),
				swapCommand: []resp.Value{
					resp.StringValue("SWAPDB"), resp.StringValue("index0"), resp.StringValue("1"),
				},
				want0:   make([]resp.Value, 0),
				want1:   make([]resp.Value, 0),
				wantErr: errors.New("both database indices must be integers"),
			},
			{
				name:         "3. Second database index is not an integer",
				presetValues: nil,
				database0:    "0",
				database1:    "index1",
				getCommand:   make([]resp.Value, 0),
				swapCommand: []resp.Value{
					resp.StringValue("SWAPDB"), resp.StringValue("0"), resp.StringValue("index1"),
				},
				want0:   make([]resp.Value, 0),
				want1:   make([]resp.Value, 0),
				wantErr: errors.New("both database indices must be integers"),
			},
			{
				name:         "4. First database index is < 0",
				presetValues: nil,
				database0:    "-1",
				database1:    "1",
				getCommand:   make([]resp.Value, 0),
				swapCommand: []resp.Value{
					resp.StringValue("SWAPDB"), resp.StringValue("-1"), resp.StringValue("1"),
				},
				want0:   make([]resp.Value, 0),
				want1:   make([]resp.Value, 0),
				wantErr: errors.New("database indices must be >= 0"),
			},
			{
				name:         "5. Second database index is < 0",
				presetValues: nil,
				database0:    "1",
				database1:    "-1",
				getCommand:   make([]resp.Value, 0),
				swapCommand: []resp.Value{
					resp.StringValue("SWAPDB"), resp.StringValue("0"), resp.StringValue("-1"),
				},
				want0:   make([]resp.Value, 0),
				want1:   make([]resp.Value, 0),
				wantErr: errors.New("database indices must be >= 0"),
			},
			{
				name:         "6. Command too short",
				presetValues: nil,
				database0:    "-1",
				database1:    "1",
				getCommand:   make([]resp.Value, 0),
				swapCommand:  []resp.Value{resp.StringValue("SWAPDB"), resp.StringValue("0")},
				want0:        make([]resp.Value, 0),
				want1:        make([]resp.Value, 0),
				wantErr:      errors.New(constants.WrongArgsResponse),
			},
			{
				name:         "7. Command too long",
				presetValues: nil,
				database0:    "-1",
				database1:    "1",
				getCommand:   make([]resp.Value, 0),
				swapCommand: []resp.Value{
					resp.StringValue("SWAPDB"), resp.StringValue("0"),
					resp.StringValue("1"), resp.StringValue("2"),
				},
				want0:   make([]resp.Value, 0),
				want1:   make([]resp.Value, 0),
				wantErr: errors.New(constants.WrongArgsResponse),
			},
		}

		for _, test := range tests {
			// Set values for database 0 and 1.
			if test.presetValues != nil {
				for db, data := range test.presetValues {
					_ = mockServer.SelectDB(db)
					if _, err = mockServer.MSet(data); err != nil {
						t.Error(err)
						return
					}
				}
			}

			// Create TPC connection for database 0
			conn1, err := internal.GetConnection("localhost", port)
			if err != nil {
				t.Error(err)
				return
			}
			client1 := resp.NewConn(conn1)
			if len(test.getCommand) > 0 {
				// Select database 0 for connection 1
				if err = client1.WriteArray([]resp.Value{
					resp.StringValue("SELECT"),
					resp.StringValue(test.database0),
				}); err != nil {
					t.Error(err)
					return
				}
				res, _, err := client1.ReadValue()
				if err != nil {
					t.Error(err)
					return
				}
				if !strings.EqualFold(res.String(), "ok") {
					t.Errorf("expcted OK response when selecting database, got %s", res.String())
					return
				}

				// Check that the connection reads values from database 0
				if err = client1.WriteArray(test.getCommand); err != nil {
					t.Error(err)
					return
				}
				res, _, err = client1.ReadValue()
				if err != nil {
					t.Error(err)
					return
				}
				if !reflect.DeepEqual(test.want0, res.Array()) {
					t.Errorf("expected response %+v, got %+v", test.want0, res.Array())
				}
			}

			// Create TCP connection for database 1
			conn2, err := internal.GetConnection("localhost", port)
			if err != nil {
				t.Error(err)
				return
			}
			client2 := resp.NewConn(conn2)
			if len(test.getCommand) > 0 {
				// Select database 1 for the second connection.
				if err = client2.WriteArray([]resp.Value{
					resp.StringValue("SELECT"),
					resp.StringValue(test.database1),
				}); err != nil {
					t.Error(err)
					return
				}
				res, _, err := client2.ReadValue()
				if err != nil {
					t.Error(err)
					return
				}
				if !strings.EqualFold(res.String(), "ok") {
					t.Errorf("expcted OK response when selecting database, got %s", res.String())
					return
				}
				// Check that the connection reads values from database 1.
				if err = client2.WriteArray(test.getCommand); err != nil {
					t.Error(err)
					return
				}
				res, _, err = client2.ReadValue()
				if err != nil {
					t.Error(err)
					return
				}
				if !reflect.DeepEqual(test.want1, res.Array()) {
					t.Errorf("expected response %+v, got %+v", test.want1, res.Array())
				}
			}

			// Run SWAPDB command
			if err = client1.WriteArray(test.swapCommand); err != nil {
				t.Error(err)
				return
			}
			res, _, err := client1.ReadValue()
			if err != nil {
				t.Error(err)
				return
			}
			// If we expect an error check the error.
			if test.wantErr != nil {
				if !strings.Contains(res.Error().Error(), test.wantErr.Error()) {
					t.Errorf("expected error response to contain \"%s\", go \"%s\"",
						test.wantErr.Error(), res.Error().Error())
				}
				continue
			}
			// Check if response is OK.
			if !strings.EqualFold(res.String(), "ok") {
				t.Errorf("expected OK response from SWAPDB command, got %s", res.String())
				return
			}

			// Check that the first connection now reads values from database 1
			if err = client1.WriteArray(test.getCommand); err != nil {
				t.Error(err)
				return
			}
			res, _, err = client1.ReadValue()
			if err != nil {
				t.Error(err)
				return
			}
			if !reflect.DeepEqual(test.want1, res.Array()) {
				t.Errorf("expected response %+v, got %+v", test.want1, res.Array())
			}

			// Check that the second connection now reads values from database 0
			if err = client2.WriteArray(test.getCommand); err != nil {
				t.Error(err)
				return
			}
			res, _, err = client2.ReadValue()
			if err != nil {
				t.Error(err)
				return
			}
			if !reflect.DeepEqual(test.want0, res.Array()) {
				t.Errorf("expected response %+v, got %+v", test.want0, res.Array())
			}
		}
	})
}
