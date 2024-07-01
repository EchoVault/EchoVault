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

package acl_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/echovault/echovault/echovault"
	"github.com/echovault/echovault/internal"
	"github.com/echovault/echovault/internal/config"
	"github.com/echovault/echovault/internal/constants"
	"github.com/tidwall/resp"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
	"testing"
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
		// If the user already exists in the server, skip.
		existingUsers, err := mockServer.ACLUsers()
		if err != nil {
			return nil, err
		}
		if slices.Contains(existingUsers, user.Username) {
			continue
		}
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

// compareSlices compare the elements in 2 slices, it checks if every element is s1 is contained in s2
// and vice versa. It essentially does a deep equality comparison.
// This is done manually rather than using slices.Equal because it would be ideal to throw an error
// specifying exactly which items are missing in either slice.
func compareSlices[T comparable](res, expected []T) error {
	if len(res) != len(expected) {
		return fmt.Errorf("expected slice of length %d, got slice of length %d", len(expected), len(res))
	}
	// Check whether all elements in res are contained in expected
	for _, r := range res {
		if !slices.Contains(expected, r) {
			return fmt.Errorf("got response item %+v, but it's not contained in expected slices", r)
		}
	}
	// Check whether all elements in expected are contained in res
	for _, e := range expected {
		if !slices.Contains(res, e) {
			return fmt.Errorf("expected element %+v, not found in res slice", e)
		}
	}
	return nil
}

// compareUsers compares 2 users and checks if all their fields are equal
func compareUsers(user1, user2 map[string][]string) error {
	// Compare flags
	if user1["username"][0] != user2["username"][0] {
		return fmt.Errorf("mismatched usernames \"%s\", and \"%s\"", user1["username"][0], user2["username"][0])
	}

	// Check if both users are enabled.
	if slices.Contains(user1["flags"], "on") != slices.Contains(user2["flags"], "on") {
		return fmt.Errorf("mismatched enabled flag \"%+v\", and \"%+v\"",
			slices.Contains(user1["flags"], "on"), slices.Contains(user2["flags"], "on"))
	}

	// Check if "nokeys" is present
	if slices.Contains(user1["flags"], "nokeys") != slices.Contains(user2["flags"], "nokeys") {
		return fmt.Errorf("mismatched nokeys flag \"%+v\", and \"%+v\"",
			slices.Contains(user1["flags"], "nokeys"), slices.Contains(user2["flags"], "nokeys"))
	}

	// Check if "nopass" is present
	if slices.Contains(user1["flags"], "nopass") != slices.Contains(user1["flags"], "nopass") {
		return fmt.Errorf("mismatched nopassword flag \"%+v\", and \"%+v\"",
			slices.Contains(user1["flags"], "nopass"), slices.Contains(user1["flags"], "nopass"))
	}

	// Compare permissions
	permissions := [][][]string{
		{user1["categories"], user2["categories"]},
		{user1["commands"], user2["commands"]},
		{user1["keys"], user2["keys"]},
		{user1["channels"], user2["channels"]},
	}
	for _, p := range permissions {
		if err := compareSlices(p[0], p[1]); err != nil {
			return err
		}
	}

	return nil
}

func generateSHA256Password(plain string) string {
	h := sha256.New()
	h.Write([]byte(plain))
	return hex.EncodeToString(h.Sum(nil))
}

func Test_ACL(t *testing.T) {
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

	t.Run("Test_Permissions", func(t *testing.T) {
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

		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		client := resp.NewConn(conn)

		t.Cleanup(func() {
			_ = conn.Close()
			mockServer.ShutDown()
		})

		// Add users to be used in test cases.
		users := []echovault.User{
			{
				// User with nokeys flag enables.
				Username:          "test_nokeys",
				Enabled:           true,
				NoKeys:            true,
				AddPlainPasswords: []string{"test_nokeys_password"},
			},
			{
				// This use will be used to test authorization failure when trying to access resources that are not
				// in their "included" rules.
				Username:          "test_included",
				Enabled:           true,
				AddPlainPasswords: []string{"test_included_password"},
				IncludeCategories: []string{
					constants.WriteCategory,
					constants.ReadCategory,
					constants.SlowCategory,
					constants.PubSubCategory,
					constants.ConnectionCategory,
					constants.ListCategory,
				},
				IncludeCommands:      []string{"set", "get", "subscribe", "lrange", "ltrim"},
				IncludeChannels:      []string{"channel[12]"},
				IncludeReadWriteKeys: []string{"key1", "key2"},
			},
			{
				// This use will be used to test authorization failure when trying to access resources that are
				// in their "excluded" rules.
				Username:          "test_excluded",
				Enabled:           true,
				AddPlainPasswords: []string{"test_excluded_password"},
				IncludeCategories: []string{"*"},
				ExcludeCategories: []string{constants.FastCategory, constants.HashCategory},
				IncludeCommands:   []string{"*"},
				ExcludeCommands:   []string{"set", "mset"},
				IncludeChannels:   []string{"*"},
				ExcludeChannels:   []string{"channel[12]"},
			},
		}
		for _, user := range users {
			if _, err := mockServer.ACLSetUser(user); err != nil {
				t.Error(err)
				return
			}
		}

		tests := []struct {
			name    string
			auth    []resp.Value
			cmd     []resp.Value
			wantErr string
		}{
			{
				name:    "1. Return error when the connection is not authenticated",
				auth:    []resp.Value{},
				cmd:     []resp.Value{resp.StringValue("SET"), resp.StringValue("key"), resp.StringValue("value")},
				wantErr: "user must be authenticated",
			},
			{
				name: "2. Return error when the command category is not in the included categories section",
				auth: []resp.Value{
					resp.StringValue("AUTH"),
					resp.StringValue("test_included"),
					resp.StringValue("test_included_password"),
				},
				cmd: []resp.Value{
					resp.StringValue("HSET"),
					resp.StringValue("hash"),
					resp.StringValue("field1"),
					resp.StringValue("value1"),
				},
				wantErr: fmt.Sprintf("unauthorized access to the following categories: [@%s @%s]",
					constants.FastCategory, constants.HashCategory),
			},
			{
				name: "3. Return error when the command category is in the excluded categories section",
				auth: []resp.Value{
					resp.StringValue("AUTH"),
					resp.StringValue("test_excluded"),
					resp.StringValue("test_excluded_password"),
				},
				cmd: []resp.Value{
					resp.StringValue("HSET"),
					resp.StringValue("hash"),
					resp.StringValue("field1"),
					resp.StringValue("value1"),
				},
				wantErr: fmt.Sprintf("unauthorized access to the following categories: [@%s]",
					constants.HashCategory),
			},
			{
				name: "4. Return error when the command is not in the included command category",
				auth: []resp.Value{
					resp.StringValue("AUTH"),
					resp.StringValue("test_included"),
					resp.StringValue("test_included_password"),
				},
				cmd: []resp.Value{
					resp.StringValue("MSET"),
					resp.StringValue("key1"),
					resp.StringValue("value1"),
				},
				wantErr: "not authorised to run MSET command",
			},
			{
				name: "5. Return error when the command is in the excluded command category",
				auth: []resp.Value{
					resp.StringValue("AUTH"),
					resp.StringValue("test_excluded"),
					resp.StringValue("test_excluded_password"),
				},
				cmd: []resp.Value{
					resp.StringValue("SET"),
					resp.StringValue("key1"),
					resp.StringValue("value1"),
				},
				wantErr: "not authorised to run SET command",
			},
			{
				name: "6. Return error when subscribing to channel that's not in included channels",
				auth: []resp.Value{
					resp.StringValue("AUTH"),
					resp.StringValue("test_included"),
					resp.StringValue("test_included_password"),
				},
				cmd: []resp.Value{
					resp.StringValue("SUBSCRIBE"),
					resp.StringValue("channel3"),
				},
				wantErr: "not authorised to access channel &channel3",
			},
			{
				name: "7. Return error when publishing to channel that's in excluded channels",
				auth: []resp.Value{
					resp.StringValue("AUTH"),
					resp.StringValue("test_excluded"),
					resp.StringValue("test_excluded_password"),
				},
				cmd: []resp.Value{
					resp.StringValue("SUBSCRIBE"),
					resp.StringValue("channel2"),
				},
				wantErr: "not authorised to access channel &channel2",
			},
			{
				name: "8. Return error when the user has nokeys flag",
				auth: []resp.Value{
					resp.StringValue("AUTH"),
					resp.StringValue("test_nokeys"),
					resp.StringValue("test_nokeys_password"),
				},
				cmd: []resp.Value{resp.StringValue("GET"), resp.StringValue("key1")},
			},
			{
				name: "9. Return error when trying to read from keys that are not in read keys list",
				auth: []resp.Value{
					resp.StringValue("AUTH"),
					resp.StringValue("test_included"),
					resp.StringValue("test_included_password"),
				},
				cmd: []resp.Value{
					resp.StringValue("LRANGE"),
					resp.StringValue("key3"),
					resp.StringValue("0"),
					resp.StringValue("-1"),
				},
				wantErr: fmt.Sprintf("not authorised to access the following read keys: [%s~%s]", "%R", "key3"),
			},
			{
				name: "10. Return error when trying to write to keys that are not in write keys list",
				auth: []resp.Value{
					resp.StringValue("AUTH"),
					resp.StringValue("test_included"),
					resp.StringValue("test_included_password"),
				},
				cmd: []resp.Value{
					resp.StringValue("LTRIM"),
					resp.StringValue("key3"),
					resp.StringValue("0"),
					resp.StringValue("3"),
				},
				wantErr: fmt.Sprintf("not authorised to access the following write keys: [%s~%s]", "%W", "key3"),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				// Authenticate the user if the auth command is provided.
				if len(test.auth) > 0 {
					err := client.WriteArray(test.auth)
					if err != nil {
						t.Error(err)
						return
					}
					res, _, err := client.ReadValue()
					if err != nil {
						t.Error(err)
						return
					}
					if !strings.EqualFold(res.String(), "ok") {
						t.Errorf("expected auth response to  be OK, got \"%s\"", res.String())
					}
				}

				if err := client.WriteArray(test.cmd); err != nil {
					t.Error(err)
					return
				}

				res, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
					return
				}

				if !strings.Contains(res.Error().Error(), test.wantErr) {
					t.Errorf("expected error to contain string \"%s\", got \"%s\"",
						test.wantErr, res.Error().Error())
					return
				}
			})
		}
	})

	t.Run("Test_HandleCat", func(t *testing.T) {
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

		// Authenticate connection
		if err = r.WriteArray([]resp.Value{resp.StringValue("AUTH"), resp.StringValue("password1")}); err != nil {
			t.Error(err)
		}
		rv, _, err := r.ReadValue()
		if err != nil {
			t.Error(err)
		}
		if rv.String() != "OK" {
			t.Error("could not authenticate user")
		}

		// Since only ACL commands are loaded in this test suite, this test will only test against the
		// list of categories and commands available in the ACL module.
		tests := []struct {
			cmd     []resp.Value
			wantRes []string
			wantErr string
		}{
			{ // 1. Return list of categories
				cmd: []resp.Value{resp.StringValue("ACL"), resp.StringValue("CAT")},
				wantRes: []string{
					constants.ConnectionCategory,
					constants.SlowCategory,
					constants.FastCategory,
					constants.AdminCategory,
					constants.DangerousCategory,
				},
				wantErr: "",
			},
			{ // 2. Return list of commands in connection category
				cmd:     []resp.Value{resp.StringValue("ACL"), resp.StringValue("CAT"), resp.StringValue(constants.ConnectionCategory)},
				wantRes: []string{"auth"},
				wantErr: "",
			},
			{ // 3. Return list of commands in slow category
				cmd:     []resp.Value{resp.StringValue("ACL"), resp.StringValue("CAT"), resp.StringValue(constants.SlowCategory)},
				wantRes: []string{"auth", "acl|cat", "acl|users", "acl|setuser", "acl|getuser", "acl|deluser", "acl|list", "acl|load", "acl|save"},
				wantErr: "",
			},
			{ // 4. Return list of commands in fast category
				cmd:     []resp.Value{resp.StringValue("ACL"), resp.StringValue("CAT"), resp.StringValue(constants.FastCategory)},
				wantRes: []string{"acl|whoami"},
				wantErr: "",
			},
			{ // 5. Return list of commands in admin category
				cmd:     []resp.Value{resp.StringValue("ACL"), resp.StringValue("CAT"), resp.StringValue(constants.AdminCategory)},
				wantRes: []string{"acl|users", "acl|setuser", "acl|getuser", "acl|deluser", "acl|list", "acl|load", "acl|save"},
				wantErr: "",
			},
			{ // 6. Return list of commands in dangerous category
				cmd:     []resp.Value{resp.StringValue("ACL"), resp.StringValue("CAT"), resp.StringValue(constants.DangerousCategory)},
				wantRes: []string{"acl|users", "acl|setuser", "acl|getuser", "acl|deluser", "acl|list", "acl|load", "acl|save"},
				wantErr: "",
			},
			{ // 7. Return error when category does not exist
				cmd:     []resp.Value{resp.StringValue("ACL"), resp.StringValue("CAT"), resp.StringValue("non-existent")},
				wantRes: nil,
				wantErr: "Error category NON-EXISTENT not found",
			},
			{ // 8. Command too long
				cmd:     []resp.Value{resp.StringValue("ACL"), resp.StringValue("CAT"), resp.StringValue("category1"), resp.StringValue("category2")},
				wantRes: nil,
				wantErr: fmt.Sprintf("Error %s", constants.WrongArgsResponse),
			},
		}

		for _, test := range tests {
			if err = r.WriteArray(test.cmd); err != nil {
				t.Error(err)
			}
			rv, _, err = r.ReadValue()
			if err != nil {
				t.Error(err)
			}
			if test.wantErr != "" {
				if rv.Error().Error() != test.wantErr {
					t.Errorf("expected error response \"%s\", got \"%s\"", test.wantErr, rv.Error().Error())
				}
				continue
			}
			resArr := rv.Array()
			// Check if all the elements in the expected array are in the response array
			for _, expected := range test.wantRes {
				if !slices.ContainsFunc(resArr, func(value resp.Value) bool {
					return value.String() == expected
				}) {
					t.Errorf("could not find expected command \"%s\" in the response array for category", expected)
				}
			}
		}
	})

	t.Run("Test_HandleUsers", func(t *testing.T) {
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

		users := []string{"default", "with_password_user", "no_password_user", "disabled_user"}

		if err = r.WriteArray([]resp.Value{resp.StringValue("ACL"), resp.StringValue("USERS")}); err != nil {
			t.Error(err)
		}

		rv, _, err := r.ReadValue()
		if err != nil {
			t.Error(err)
		}

		resArr := rv.Array()

		// Check if all the expected users are in the response array
		for _, user := range users {
			if !slices.ContainsFunc(resArr, func(value resp.Value) bool {
				return value.String() == user
			}) {
				t.Errorf("could not find expected user \"%s\" in response array", user)
			}
		}

		// Check if all the users in the response array are in the expected users
		for _, value := range resArr {
			if !slices.ContainsFunc(users, func(user string) bool {
				return value.String() == user
			}) {
				t.Errorf("could not find response user \"%s\" in expected users array", value.String())
			}
		}
	})

	t.Run("Test_HandleSetUser", func(t *testing.T) {
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

		t.Cleanup(func() {
			mockServer.ShutDown()
		})

		tests := []struct {
			name       string
			presetUser *echovault.User
			cmd        []resp.Value
			wantRes    string
			wantErr    string
			wantUser   map[string][]string
		}{
			{
				name:       "1. Create new enabled user",
				presetUser: nil,
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("SETUSER"),
					resp.StringValue("set_user_1"),
					resp.StringValue("on"),
				},
				wantRes: "OK",
				wantErr: "",
				wantUser: map[string][]string{
					"username":   {"set_user_1"},
					"flags":      {"on"},
					"categories": {"+@all"},
					"commands":   {"+all"},
					"keys":       {"%RW~*"},
					"channels":   {"+&*"},
				},
			},
			{
				name:       "2. Create new disabled user",
				presetUser: nil,
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("SETUSER"),
					resp.StringValue("set_user_2"),
					resp.StringValue("off"),
				},
				wantRes: "OK",
				wantErr: "",
				wantUser: map[string][]string{
					"username":   {"set_user_2"},
					"flags":      {"off"},
					"categories": {"+@all"},
					"commands":   {"+all"},
					"keys":       {"%RW~*"},
					"channels":   {"+&*"},
				},
			},
			{
				name:       "3. Create new enabled user with both plaintext and SHA256 passwords",
				presetUser: nil,
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("SETUSER"),
					resp.StringValue("set_user_3"),
					resp.StringValue("on"),
					resp.StringValue(">set_user_3_plaintext_password_1"),
					resp.StringValue(">set_user_3_plaintext_password_2"),
					resp.StringValue(fmt.Sprintf("#%s", generateSHA256Password("set_user_3_hash_password_1"))),
					resp.StringValue(fmt.Sprintf("#%s", generateSHA256Password("set_user_3_hash_password_2"))),
				},
				wantRes: "OK",
				wantErr: "",
				wantUser: map[string][]string{
					"username":   {"set_user_3"},
					"flags":      {"on"},
					"categories": {"+@all"},
					"commands":   {"+all"},
					"keys":       {"%RW~*"},
					"channels":   {"+&*"},
				},
			},
			{
				name: "4. Remove plaintext and SHA256 password from existing user",
				presetUser: &echovault.User{
					Username:          "set_user_4",
					Enabled:           true,
					AddPlainPasswords: []string{"set_user_4_plaintext_password_1", "set_user_4_plaintext_password_2"},
					AddHashPasswords: []string{
						generateSHA256Password("set_user_4_hash_password_1"),
						generateSHA256Password("set_user_4_hash_password_2"),
					},
				},
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("SETUSER"),
					resp.StringValue("set_user_4"),
					resp.StringValue("on"),
					resp.StringValue("<set_user_3_plaintext_password_2"),
					resp.StringValue(fmt.Sprintf("!%s", generateSHA256Password("set_user_3_hash_password_2"))),
				},
				wantRes: "OK",
				wantErr: "",
				wantUser: map[string][]string{
					"username":   {"set_user_4"},
					"flags":      {"on"},
					"categories": {"+@all"},
					"commands":   {"+all"},
					"keys":       {"%RW~*"},
					"channels":   {"+&*"},
				},
			},
			{
				name:       "5. Create user with no commands allowed to be executed",
				presetUser: nil,
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("SETUSER"),
					resp.StringValue("set_user_5"),
					resp.StringValue("on"),
					resp.StringValue("nocommands"),
				},
				wantRes: "OK",
				wantErr: "",
				wantUser: map[string][]string{
					"username":   {"set_user_5"},
					"flags":      {"on"},
					"categories": {"-@all"},
					"commands":   {"-all"},
					"keys":       {"%RW~*"},
					"channels":   {"+&*"},
				},
			},
			{
				name:       "6. Create user that can access all categories with +@*",
				presetUser: nil,
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("SETUSER"),
					resp.StringValue("set_user_6"),
					resp.StringValue("on"),
					resp.StringValue("+@*"),
				},
				wantRes: "OK",
				wantErr: "",
				wantUser: map[string][]string{
					"username":   {"set_user_6"},
					"flags":      {"on"},
					"categories": {"+@all"},
					"commands":   {"+all"},
					"keys":       {"%RW~*"},
					"channels":   {"+&*"},
				},
			},
			{
				name:       "7. Create user that can access all categories with allcategories flag",
				presetUser: nil,
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("SETUSER"),
					resp.StringValue("set_user_7"),
					resp.StringValue("on"),
					resp.StringValue("allcategories"),
				},
				wantRes: "OK",
				wantErr: "",
				wantUser: map[string][]string{
					"username":   {"set_user_7"},
					"flags":      {"on"},
					"categories": {"+@all"},
					"commands":   {"+all"},
					"keys":       {"%RW~*"},
					"channels":   {"+&*"},
				},
			},
			{
				name:       "8. Create user with a few allowed categories and a few disallowed categories",
				presetUser: nil,
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("SETUSER"),
					resp.StringValue("set_user_8"),
					resp.StringValue("on"),
					resp.StringValue(fmt.Sprintf("+@%s", constants.WriteCategory)),
					resp.StringValue(fmt.Sprintf("+@%s", constants.ReadCategory)),
					resp.StringValue(fmt.Sprintf("+@%s", constants.PubSubCategory)),
					resp.StringValue(fmt.Sprintf("-@%s", constants.AdminCategory)),
					resp.StringValue(fmt.Sprintf("-@%s", constants.ConnectionCategory)),
					resp.StringValue(fmt.Sprintf("-@%s", constants.DangerousCategory)),
				},
				wantRes: "OK",
				wantErr: "",
				wantUser: map[string][]string{
					"username": {"set_user_8"},
					"flags":    {"on"},
					"categories": {
						fmt.Sprintf("+@%s", constants.WriteCategory),
						fmt.Sprintf("+@%s", constants.ReadCategory),
						fmt.Sprintf("+@%s", constants.PubSubCategory),
						fmt.Sprintf("-@%s", constants.AdminCategory),
						fmt.Sprintf("-@%s", constants.ConnectionCategory),
						fmt.Sprintf("-@%s", constants.DangerousCategory),
					},
					"commands": {"+all"},
					"keys":     {"%RW~*"},
					"channels": {"+&*"},
				},
			},
			{
				name:       "9. Create user that is not allowed to access any keys",
				presetUser: nil,
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("SETUSER"),
					resp.StringValue("set_user_9"),
					resp.StringValue("on"),
					resp.StringValue("resetkeys"),
				},
				wantRes: "OK",
				wantErr: "",
				wantUser: map[string][]string{
					"username":   {"set_user_9"},
					"flags":      {"on", "nokeys"},
					"categories": {"+@all"},
					"commands":   {"+all"},
					"keys":       {},
					"channels":   {"+&*"},
				},
			},
			{
				name: `10. Create user that can access some read keys and some write keys.
	Provide keys that are RW, W-Only and R-Only`,
				presetUser: nil,
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("SETUSER"),
					resp.StringValue("set_user_10"),
					resp.StringValue("on"),
					resp.StringValue("%RW~key1"),
					resp.StringValue("%RW~key2"),
					resp.StringValue("%RW~key3"),
					resp.StringValue("%RW~key4"),
					resp.StringValue("%R~key5"),
					resp.StringValue("%R~key6"),
					resp.StringValue("%W~key7"),
					resp.StringValue("%W~key8"),
				},
				wantRes: "OK",
				wantErr: "",
				wantUser: map[string][]string{
					"username":   {"set_user_10"},
					"flags":      {"on"},
					"categories": {"+@all"},
					"commands":   {"+all"},
					"keys":       {"%RW~key1", "%RW~key2", "%RW~key3", "%RW~key4", "%R~key5", "%R~key6", "%W~key7", "%W~key8"},
					"channels":   {"+&*"},
				},
			},
			{
				name:       "11. Create user that can access all pubsub channels with +&*",
				presetUser: nil,
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("SETUSER"),
					resp.StringValue("set_user_11"),
					resp.StringValue("on"),
					resp.StringValue("+&*"),
				},
				wantRes: "OK",
				wantErr: "",
				wantUser: map[string][]string{
					"username":   {"set_user_11"},
					"flags":      {"on"},
					"categories": {"+@all"},
					"commands":   {"+all"},
					"keys":       {"%RW~*"},
					"channels":   {"+&*"},
				},
			},
			{
				name:       "12. Create user that can access all pubsub channels with allchannels flag",
				presetUser: nil,
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("SETUSER"),
					resp.StringValue("set_user_12"),
					resp.StringValue("on"),
					resp.StringValue("allchannels"),
				},
				wantRes: "OK",
				wantErr: "",
				wantUser: map[string][]string{
					"username":   {"set_user_12"},
					"flags":      {"on"},
					"categories": {"+@all"},
					"commands":   {"+all"},
					"keys":       {"%RW~*"},
					"channels":   {"+&*"},
				},
			},
			{
				name:       "13. Create user with a few allowed pubsub channels and a few disallowed channels",
				presetUser: nil,
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("SETUSER"),
					resp.StringValue("set_user_13"),
					resp.StringValue("on"),
					resp.StringValue("+&channel1"),
					resp.StringValue("+&channel2"),
					resp.StringValue("-&channel3"),
					resp.StringValue("-&channel4"),
				},
				wantRes: "OK",
				wantErr: "",
				wantUser: map[string][]string{
					"username":   {"set_user_13"},
					"flags":      {"on"},
					"categories": {"+@all"},
					"commands":   {"+all"},
					"keys":       {"%RW~*"},
					"channels":   {"+&channel1", "+&channel2", "-&channel3", "-&channel4"},
				},
			},
			{
				name:       "14. Create user that can access all commands",
				presetUser: nil,
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("SETUSER"),
					resp.StringValue("set_user_14"),
					resp.StringValue("on"),
					resp.StringValue("allcommands"),
				},
				wantRes: "OK",
				wantErr: "",
				wantUser: map[string][]string{
					"username":   {"set_user_14"},
					"flags":      {"on"},
					"categories": {"+@all"},
					"commands":   {"+all"},
					"keys":       {"%RW~*"},
					"channels":   {"+&*"},
				},
			},
			{
				name:       "15. Create user with some allowed commands and disallowed commands",
				presetUser: nil,
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("SETUSER"),
					resp.StringValue("set_user_15"),
					resp.StringValue("on"),
					resp.StringValue("+acl|getuser"),
					resp.StringValue("+acl|setuser"),
					resp.StringValue("+acl|deluser"),
					resp.StringValue("-rewriteaof"),
					resp.StringValue("-save"),
					resp.StringValue("-publish"),
				},
				wantRes: "OK",
				wantErr: "",
				wantUser: map[string][]string{
					"username":   {"set_user_15"},
					"flags":      {"on"},
					"categories": {"+@all"},
					"commands":   {"+acl|getuser", "+acl|setuser", "+acl|deluser", "-rewriteaof", "-save", "-publish"},
					"keys":       {"%RW~*"},
					"channels":   {"+&*"},
				},
			},
			{
				name: `16. Create new user with no password using 'nopass'.
	When nopass is provided, ignore any passwords that may have been provided in the command.`,
				presetUser: nil,
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("SETUSER"),
					resp.StringValue("set_user_16"),
					resp.StringValue("on"),
					resp.StringValue("nopass"),
					resp.StringValue(">password1"),
					resp.StringValue(fmt.Sprintf("#%s", generateSHA256Password("password2"))),
				},
				wantRes: "OK",
				wantErr: "",
				wantUser: map[string][]string{
					"username":   {"set_user_16"},
					"flags":      {"on", "nopass"},
					"categories": {"+@all"},
					"commands":   {"+all"},
					"keys":       {"%RW~*"},
					"channels":   {"+&*"},
				},
			},
			{
				name: "17. Delete all existing users passwords using 'nopass'",
				presetUser: &echovault.User{
					Username:          "set_user_17",
					Enabled:           true,
					NoPassword:        true,
					AddPlainPasswords: []string{"password1"},
					AddHashPasswords:  []string{generateSHA256Password("password2")},
				},
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("SETUSER"),
					resp.StringValue("set_user_17"),
					resp.StringValue("on"),
					resp.StringValue("nopass"),
				},
				wantRes: "OK",
				wantErr: "",
				wantUser: map[string][]string{
					"username":   {"set_user_17"},
					"flags":      {"on", "nopass"},
					"categories": {"+@all"},
					"commands":   {"+all"},
					"keys":       {"%RW~*"},
					"channels":   {"+&*"},
				},
			},
			{
				name: "18. Clear all of an existing user's passwords using 'resetpass'",
				presetUser: &echovault.User{
					Username:          "set_user_18",
					Enabled:           true,
					NoPassword:        true,
					AddPlainPasswords: []string{"password1"},
					AddHashPasswords:  []string{generateSHA256Password("password2")},
				},
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("SETUSER"),
					resp.StringValue("set_user_18"),
					resp.StringValue("on"),
					resp.StringValue("nopass"),
				},
				wantRes: "OK",
				wantErr: "",
				wantUser: map[string][]string{
					"username":   {"set_user_18"},
					"flags":      {"on", "nopass"},
					"categories": {"+@all"},
					"commands":   {"+all"},
					"keys":       {"%RW~*"},
					"channels":   {"+&*"},
				},
			},
			{
				name: "19. Clear all of an existing user's command privileges using 'nocommands'",
				presetUser: &echovault.User{
					Username:        "set_user_19",
					Enabled:         true,
					IncludeCommands: []string{"acl|getuser", "acl|setuser", "acl|deluser"},
					ExcludeCommands: []string{"rewriteaof", "save"},
				},
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("SETUSER"),
					resp.StringValue("set_user_19"),
					resp.StringValue("on"),
					resp.StringValue("nocommands"),
				},
				wantRes: "OK",
				wantErr: "",
				wantUser: map[string][]string{
					"username":   {"set_user_19"},
					"flags":      {"on"},
					"categories": {"-@all"},
					"commands":   {"-all"},
					"keys":       {"%RW~*"},
					"channels":   {"+&*"},
				},
			},
			{
				name: "20. Clear all of an existing user's allowed keys using 'resetkeys'",
				presetUser: &echovault.User{
					Username:         "set_user_20",
					Enabled:          true,
					IncludeWriteKeys: []string{"key1", "key2", "key3", "key4", "key5", "key6"},
					IncludeReadKeys:  []string{"key1", "key2", "key3", "key7", "key8", "key9"},
				},
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("SETUSER"),
					resp.StringValue("set_user_20"),
					resp.StringValue("on"),
					resp.StringValue("resetkeys"),
				},
				wantRes: "OK",
				wantErr: "",
				wantUser: map[string][]string{
					"username":   {"set_user_20"},
					"flags":      {"on", "nokeys"},
					"categories": {"+@all"},
					"commands":   {"+all"},
					"keys":       {},
					"channels":   {"+&*"},
				},
			},
			{
				name: "21. Allow user to access all channels using 'resetchannels'",
				presetUser: &echovault.User{
					Username:        "set_user_21",
					Enabled:         true,
					IncludeChannels: []string{"channel1", "channel2"},
					ExcludeChannels: []string{"channel3", "channel4"},
				},
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("SETUSER"),
					resp.StringValue("set_user_21"),
					resp.StringValue("resetchannels"),
				},
				wantRes: "OK",
				wantErr: "",
				wantUser: map[string][]string{
					"username":   {"set_user_21"},
					"flags":      {"on"},
					"categories": {"+@all"},
					"commands":   {"+all"},
					"keys":       {"%RW~*"},
					"channels":   {"-&*"},
				},
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetUser != nil {
					if _, err := mockServer.ACLSetUser(*test.presetUser); err != nil {
						t.Error(err)
						return
					}
				}
				if err = r.WriteArray(test.cmd); err != nil {
					t.Error(err)
				}
				v, _, err := r.ReadValue()
				if err != nil {
					t.Error(err)
				}
				if test.wantErr != "" {
					if v.Error().Error() != test.wantErr {
						t.Errorf("expected error response \"%s\", got \"%s\"", test.wantErr, v.Error().Error())
					}
					return
				}
				if v.String() != test.wantRes {
					t.Errorf("expected response \"%s\", got \"%s\"", test.wantRes, v.String())
				}
				if test.wantUser == nil {
					return
				}

				user, err := mockServer.ACLGetUser(test.wantUser["username"][0])
				if err != nil {
					t.Error(err)
					return
				}

				if err = compareUsers(test.wantUser, user); err != nil {
					t.Error(err)
					return
				}
			})
		}
	})

	t.Run("Test_HandleGetUser", func(t *testing.T) {
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
			name       string
			presetUser *echovault.User
			cmd        []resp.Value
			wantRes    []resp.Value
			wantErr    string
		}{
			{
				name: "1. Get the user and all their details",
				presetUser: &echovault.User{
					Username:          "get_user_1",
					Enabled:           true,
					NoPassword:        false,
					NoKeys:            false,
					AddPlainPasswords: []string{"get_user_password_1"},
					AddHashPasswords:  []string{generateSHA256Password("get_user_password_2")},
					IncludeCategories: []string{constants.WriteCategory, constants.ReadCategory, constants.PubSubCategory},
					ExcludeCategories: []string{constants.AdminCategory, constants.ConnectionCategory, constants.DangerousCategory},
					IncludeCommands:   []string{"acl|setuser", "acl|getuser", "acl|deluser"},
					ExcludeCommands:   []string{"rewriteaof", "save", "acl|load", "acl|save"},
					IncludeReadKeys:   []string{"key1", "key2", "key3", "key4"},
					IncludeWriteKeys:  []string{"key1", "key2", "key5", "key6"},
					IncludeChannels:   []string{"channel1", "channel2"},
					ExcludeChannels:   []string{"channel3", "channel4"},
				},
				cmd: []resp.Value{resp.StringValue("ACL"), resp.StringValue("GETUSER"), resp.StringValue("get_user_1")},
				wantRes: []resp.Value{
					resp.StringValue("username"),
					resp.ArrayValue([]resp.Value{resp.StringValue("get_user_1")}),
					resp.StringValue("flags"),
					resp.ArrayValue([]resp.Value{
						resp.StringValue("on"),
					}),
					resp.StringValue("categories"),
					resp.ArrayValue([]resp.Value{
						resp.StringValue(fmt.Sprintf("+@%s", constants.WriteCategory)),
						resp.StringValue(fmt.Sprintf("+@%s", constants.ReadCategory)),
						resp.StringValue(fmt.Sprintf("+@%s", constants.PubSubCategory)),
						resp.StringValue(fmt.Sprintf("-@%s", constants.AdminCategory)),
						resp.StringValue(fmt.Sprintf("-@%s", constants.ConnectionCategory)),
						resp.StringValue(fmt.Sprintf("-@%s", constants.DangerousCategory)),
					}),
					resp.StringValue("commands"),
					resp.ArrayValue([]resp.Value{
						resp.StringValue("+acl|setuser"),
						resp.StringValue("+acl|getuser"),
						resp.StringValue("+acl|deluser"),
						resp.StringValue("-rewriteaof"),
						resp.StringValue("-save"),
						resp.StringValue("-acl|load"),
						resp.StringValue("-acl|save"),
					}),
					resp.StringValue("keys"),
					resp.ArrayValue([]resp.Value{
						// Keys here
						resp.StringValue("%RW~key1"),
						resp.StringValue("%RW~key2"),
						resp.StringValue("%R~key3"),
						resp.StringValue("%R~key4"),
						resp.StringValue("%W~key5"),
						resp.StringValue("%W~key6"),
					}),
					resp.StringValue("channels"),
					resp.ArrayValue([]resp.Value{
						// Channels here
						resp.StringValue("+&channel1"),
						resp.StringValue("+&channel2"),
						resp.StringValue("-&channel3"),
						resp.StringValue("-&channel4"),
					}),
				},
				wantErr: "",
			},
			{
				name:       "2. Return user not found error",
				presetUser: nil,
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("GETUSER"),
					resp.StringValue("non_existent_user")},
				wantRes: nil,
				wantErr: "Error user not found",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetUser != nil {
					if _, err := mockServer.ACLSetUser(*test.presetUser); err != nil {
						t.Error(err)
						return
					}
				}
				if err = r.WriteArray(test.cmd); err != nil {
					t.Error(err)
				}
				v, _, err := r.ReadValue()
				if err != nil {
					t.Error(err)
				}
				if test.wantErr != "" {
					if v.Error().Error() != test.wantErr {
						t.Errorf("expected error response \"%s\", got \"%s\"", test.wantErr, v.Error().Error())
					}
					return
				}
				resArr := v.Array()
				for i := 0; i < len(resArr); i++ {
					if slices.Contains([]string{"username", "flags", "categories", "commands", "keys", "channels"}, resArr[i].String()) {
						// String item
						if resArr[i].String() != test.wantRes[i].String() {
							t.Errorf("expected response component %+v, got %+v", test.wantRes[i], resArr[i])
						}
					} else {
						// Array item
						var expected []string
						for _, item := range test.wantRes[i].Array() {
							expected = append(expected, item.String())
						}

						var res []string
						for _, item := range resArr[i].Array() {
							res = append(res, item.String())
						}

						if err = compareSlices(res, expected); err != nil {
							t.Error(err)
						}
					}
				}
			})
		}
	})

	t.Run("Test_HandleDelUser", func(t *testing.T) {
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
			name       string
			presetUser *echovault.User
			cmd        []resp.Value
			wantRes    string
			wantErr    string
		}{
			{
				name: "1. Delete existing user while skipping default user and non-existent user",
				presetUser: &echovault.User{
					Username: "user_to_delete",
					Enabled:  true,
				},
				cmd: []resp.Value{
					resp.StringValue("ACL"),
					resp.StringValue("DELUSER"),
					resp.StringValue("default"),
					resp.StringValue("user_to_delete"),
					resp.StringValue("non_existent_user"),
				},
				wantRes: "OK",
				wantErr: "",
			},
			{
				name:       "2. Command too short",
				presetUser: nil,
				cmd:        []resp.Value{resp.StringValue("ACL"), resp.StringValue("DELUSER")},
				wantRes:    "",
				wantErr:    fmt.Sprintf("Error %s", constants.WrongArgsResponse),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetUser != nil {
					if _, err := mockServer.ACLSetUser(*test.presetUser); err != nil {
						t.Error(err)
						return
					}
				}
				if err = r.WriteArray(test.cmd); err != nil {
					t.Error(err)
				}
				v, _, err := r.ReadValue()
				if err != nil {
					t.Error(err)
				}
				if test.wantErr != "" {
					if v.Error().Error() != test.wantErr {
						t.Errorf("expected error response \"%s\", got \"%s\"", test.wantErr, v.Error().Error())
					}
					return
				}

				usernames, err := mockServer.ACLUsers()
				if err != nil {
					t.Error(err)
					return
				}

				// Check that default user still exists in the list of users
				if !slices.Contains(usernames, "default") {
					t.Error("could not find user with username \"default\" in the ACL after deleting user")
					return
				}

				// Check that the deleted user is no longer in the list
				if slices.Contains(usernames, "user_to_delete") {
					t.Error("deleted user found in the ACL")
					return
				}
			})
		}
	})

	t.Run("Test_HandleWhoAmI", func(t *testing.T) {
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
			name     string
			username string
			password string
			wantRes  string
		}{
			{
				name:     "1. With default user",
				username: "default",
				password: "password1",
				wantRes:  "default",
			},
			{
				name:     "2. With user authenticated by plaintext password",
				username: "with_password_user",
				password: "password2",
				wantRes:  "with_password_user",
			},
			{
				name:     "3. With user authenticated by SHA256 password",
				username: "with_password_user",
				password: "password3",
				wantRes:  "with_password_user",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				// Authenticate
				if err = r.WriteArray([]resp.Value{
					resp.StringValue("AUTH"),
					resp.StringValue(test.username),
					resp.StringValue(test.password),
				}); err != nil {
					t.Error(err)
				}
				v, _, err := r.ReadValue()
				if err != nil {
					t.Error(err)
				}
				if v.String() != "OK" {
					t.Errorf("expected response for auth with %s:%s to be \"OK\", got %s", test.username, test.password, v.String())
				}
				// Check whoami response value
				if err = r.WriteArray([]resp.Value{resp.StringValue("ACL"), resp.StringValue("WHOAMI")}); err != nil {
					t.Error(err)
				}
				v, _, err = r.ReadValue()
				if err != nil {
					t.Error(err)
				}
				if v.String() != test.wantRes {
					t.Errorf("expected whoami response to be \"%s\", got \"%s\"", test.wantRes, v.String())
				}
			})
		}
	})

	t.Run("Test_HandleList", func(t *testing.T) {
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
			name        string
			presetUsers []*echovault.User
			cmd         []resp.Value
			wantRes     []string
			wantErr     string
		}{
			{
				name: "1. Get the user and all their details",
				presetUsers: []*echovault.User{
					{
						Username:          "list_user_1",
						Enabled:           true,
						NoPassword:        false,
						NoKeys:            false,
						AddPlainPasswords: []string{"list_user_password_1"},
						AddHashPasswords:  []string{generateSHA256Password("list_user_password_2")},
						IncludeCategories: []string{constants.WriteCategory, constants.ReadCategory, constants.PubSubCategory},
						ExcludeCategories: []string{constants.AdminCategory, constants.ConnectionCategory, constants.DangerousCategory},
						IncludeCommands:   []string{"acl|setuser", "acl|getuser", "acl|deluser"},
						ExcludeCommands:   []string{"rewriteaof", "save", "acl|load", "acl|save"},
						IncludeReadKeys:   []string{"key1", "key2", "key3", "key4"},
						IncludeWriteKeys:  []string{"key1", "key2", "key5", "key6"},
						IncludeChannels:   []string{"channel1", "channel2"},
						ExcludeChannels:   []string{"channel3", "channel4"},
					},
					{
						Username:          "list_user_2",
						Enabled:           true,
						NoPassword:        true,
						NoKeys:            true,
						IncludeCategories: []string{constants.WriteCategory, constants.ReadCategory, constants.PubSubCategory},
						ExcludeCategories: []string{constants.AdminCategory, constants.ConnectionCategory, constants.DangerousCategory},
						IncludeCommands:   []string{"acl|setuser", "acl|getuser", "acl|deluser"},
						ExcludeCommands:   []string{"rewriteaof", "save", "acl|load", "acl|save"},
						IncludeReadKeys:   []string{},
						IncludeWriteKeys:  []string{},
						IncludeChannels:   []string{"channel1", "channel2"},
						ExcludeChannels:   []string{"channel3", "channel4"},
					},
					{
						Username:          "list_user_3",
						Enabled:           true,
						NoPassword:        false,
						NoKeys:            false,
						AddPlainPasswords: []string{"list_user_password_3"},
						AddHashPasswords:  []string{generateSHA256Password("list_user_password_4")},
						IncludeCategories: []string{constants.WriteCategory, constants.ReadCategory, constants.PubSubCategory},
						ExcludeCategories: []string{constants.AdminCategory, constants.ConnectionCategory, constants.DangerousCategory},
						IncludeCommands:   []string{"acl|setuser", "acl|getuser", "acl|deluser"},
						ExcludeCommands:   []string{"rewriteaof", "save", "acl|load", "acl|save"},
						IncludeReadKeys:   []string{"key1", "key2", "key3", "key4"},
						IncludeWriteKeys:  []string{"key1", "key2", "key5", "key6"},
						IncludeChannels:   []string{"channel1", "channel2"},
						ExcludeChannels:   []string{"channel3", "channel4"},
					},
				},
				cmd: []resp.Value{resp.StringValue("ACL"), resp.StringValue("LIST")},
				wantRes: []string{
					"default on +@all +all %RW~* +&*",
					fmt.Sprintf("with_password_user on >password2 #%s +@all +all %s~* +&*",
						generateSHA256Password("password3"), "%RW"),
					"no_password_user on nopass +@all +all %RW~* +&*",
					"disabled_user off >password5 +@all +all %RW~* +&*",
					fmt.Sprintf(`list_user_1 on >list_user_password_1 #%s +@write +@read +@pubsub -@admin -@connection -@dangerous +acl|setuser +acl|getuser +acl|deluser -rewriteaof -save -acl|load -acl|save %s +&channel1 +&channel2 -&channel3 -&channel4`,
						generateSHA256Password("list_user_password_2"), "%RW~key1 %RW~key2 %R~key3 %R~key4 %W~key5 %W~key6"),
					fmt.Sprintf(`list_user_2 on nopass nokeys +@write +@read +@pubsub -@admin -@connection -@dangerous +acl|setuser +acl|getuser +acl|deluser -rewriteaof -save -acl|load -acl|save +&channel1 +&channel2 -&channel3 -&channel4`),
					fmt.Sprintf(`list_user_3 on >list_user_password_3 #%s +@write +@read +@pubsub -@admin -@connection -@dangerous +acl|setuser +acl|getuser +acl|deluser -rewriteaof -save -acl|load -acl|save %s +&channel1 +&channel2 -&channel3 -&channel4`,
						generateSHA256Password("list_user_password_4"), "%RW~key1 %RW~key2 %R~key3 %R~key4 %W~key5 %W~key6"),
				},
				wantErr: "",
			},
			{
				name:    "2. Command too long",
				cmd:     []resp.Value{resp.StringValue("ACL"), resp.StringValue("LIST"), resp.StringValue("USERNAME")},
				wantRes: nil,
				wantErr: constants.WrongArgsResponse,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.presetUsers != nil {
					for _, user := range test.presetUsers {
						if _, err := mockServer.ACLSetUser(*user); err != nil {
							t.Error(err)
							return
						}
					}
				}

				if err = r.WriteArray(test.cmd); err != nil {
					t.Error(err)
					return
				}
				v, _, err := r.ReadValue()
				if err != nil {
					t.Error(err)
					return
				}
				if test.wantErr != "" {
					if !strings.Contains(v.Error().Error(), test.wantErr) {
						t.Errorf("expected error response \"%s\", got \"%s\"", test.wantErr, v.Error().Error())
					}
					return
				}
				resArr := v.Array()
				if len(resArr) != len(test.wantRes) {
					t.Errorf("expected response of lenght %d, got lenght %d", len(test.wantRes), len(resArr))
					return
				}

				var resStr []string
				for i := 0; i < len(resArr); i++ {
					resStr = strings.Split(resArr[i].String(), " ")
					if !slices.ContainsFunc(test.wantRes, func(s string) bool {
						expectedUserSlice := strings.Split(s, " ")
						return compareSlices(resStr, expectedUserSlice) == nil
					}) {
						t.Errorf("could not find the following user in expected slice: %+v", resStr)
						return
					}
				}
			})
		}
	})

	t.Run("Test_HandleSave", func(t *testing.T) {
		t.Parallel()

		baseDir := path.Join(".", "testdata", "save")

		tests := []struct {
			name string
			path string
			want []string // Response from ACL List command.
		}{
			{
				name: "1. Save ACL config to .json file",
				path: path.Join(baseDir, "json_test.json"),
				want: []string{
					"default on +@all +all %RW~* +&*",
					fmt.Sprintf("with_password_user on >password2 #%s +@all +all %s~* +&*",
						generateSHA256Password("password3"), "%RW"),
					"no_password_user on nopass +@all +all %RW~* +&*",
					"disabled_user off >password5 +@all +all %RW~* +&*",
				},
			},
			{
				name: "2. Save ACL config to .yaml file",
				path: path.Join(baseDir, "yaml_test.yaml"),
				want: []string{
					"default on +@all +all %RW~* +&*",
					fmt.Sprintf("with_password_user on >password2 #%s +@all +all %s~* +&*",
						generateSHA256Password("password3"), "%RW"),
					"no_password_user on nopass +@all +all %RW~* +&*",
					"disabled_user off >password5 +@all +all %RW~* +&*",
				},
			},
			{
				name: "3. Save ACL config to .yml file",
				path: path.Join(baseDir, "yml_test.yml"),
				want: []string{
					"default on +@all +all %RW~* +&*",
					fmt.Sprintf("with_password_user on >password2 #%s +@all +all %s~* +&*",
						generateSHA256Password("password3"), "%RW"),
					"no_password_user on nopass +@all +all %RW~* +&*",
					"disabled_user off >password5 +@all +all %RW~* +&*",
				},
			},
		}

		servers := make([]*echovault.EchoVault, len(tests))
		mut := sync.Mutex{}
		t.Cleanup(func() {
			_ = os.RemoveAll(baseDir)
			for _, server := range servers {
				if server != nil {
					server.ShutDown()
				}
			}
		})

		for i, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()
				mut.Lock()
				defer mut.Unlock()
				// Get free port.
				port, err := internal.GetFreePort()
				if err != nil {
					t.Error(err)
					return
				}

				// Create new server instance
				mockServer, err := setUpServer(port, false, test.path)
				if err != nil {
					t.Error(err)
					return
				}
				servers[i] = mockServer
				go func() {
					mockServer.Start()
				}()

				conn, err := internal.GetConnection("localhost", port)
				if err != nil {
					t.Error(err)
					return
				}
				client := resp.NewConn(conn)

				if err = client.WriteArray([]resp.Value{resp.StringValue("ACL"), resp.StringValue("SAVE")}); err != nil {
					t.Error(err)
					return
				}

				res, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
					return
				}

				if !strings.EqualFold(res.String(), "ok") {
					t.Errorf("expected OK response, got \"%s\"", res.String())
					return
				}

				// Close client connection
				if err = conn.Close(); err != nil {
					t.Error(err)
					return
				}

				// Shutdown the mock server
				mockServer.ShutDown()

				// Restart server and create new client connection
				port, err = internal.GetFreePort()
				if err != nil {
					t.Error(err)
					return
				}
				mockServer, err = setUpServer(port, false, test.path)
				if err != nil {
					t.Error(err)
					return
				}
				go func() {
					mockServer.Start()
				}()

				conn, err = internal.GetConnection("localhost", port)
				if err != nil {
					t.Error(err)
					return
				}
				client = resp.NewConn(conn)

				if err = client.WriteArray([]resp.Value{resp.StringValue("ACL"), resp.StringValue("LIST")}); err != nil {
					t.Error(err)
					return
				}

				res, _, err = client.ReadValue()
				if err != nil {
					t.Error(err)
					return
				}

				// Check if ACL LIST returns the expected list of users.
				resArr := res.Array()
				if len(resArr) != len(test.want) {
					t.Errorf("expected response of lenght %d, got length %d", len(test.want), len(resArr))
					return
				}

				fmt.Println("USER LIST: ")
				for j, user := range resArr {
					fmt.Printf("%d) %+v\n", j, user)
				}

				var resStr []string
				for i := 0; i < len(resArr); i++ {
					resStr = strings.Split(resArr[i].String(), " ")
					if !slices.ContainsFunc(test.want, func(s string) bool {
						expectedUserSlice := strings.Split(s, " ")
						return compareSlices(resStr, expectedUserSlice) == nil
					}) {
						t.Errorf("could not find the following user in expected slice: %+v", resStr)
						return
					}
				}
			})
		}
	})

	t.Run("Test_HandleLoad", func(t *testing.T) {
		t.Parallel()

		baseDir := path.Join(".", "testdata", "load")

		tests := []struct {
			name  string
			path  string
			users []echovault.User // Add users after server startup.
			cmd   []resp.Value     // Command to load users from ACL config.
			want  []string
		}{
			{
				name: "1. Load config from the .json file",
				path: path.Join(baseDir, "json_test.json"),
				users: []echovault.User{
					{Username: "user1", Enabled: true},
				},
				cmd: []resp.Value{resp.StringValue("ACL"), resp.StringValue("LOAD"), resp.StringValue("REPLACE")},
				want: []string{
					"default on +@all +all %RW~* +&*",
					fmt.Sprintf("with_password_user on >password2 #%s +@all +all %s~* +&*",
						generateSHA256Password("password3"), "%RW"),
					"no_password_user on nopass +@all +all %RW~* +&*",
					"disabled_user off >password5 +@all +all %RW~* +&*",
					"user1 on +@all +all %RW~* +&*",
				},
			},
			{
				name: "2. Load users from the .yaml file",
				path: path.Join(baseDir, "yaml_test.yaml"),
				users: []echovault.User{
					{Username: "user1", Enabled: true},
				},
				cmd: []resp.Value{resp.StringValue("ACL"), resp.StringValue("LOAD"), resp.StringValue("REPLACE")},
				want: []string{
					"default on +@all +all %RW~* +&*",
					fmt.Sprintf("with_password_user on >password2 #%s +@all +all %s~* +&*",
						generateSHA256Password("password3"), "%RW"),
					"no_password_user on nopass +@all +all %RW~* +&*",
					"disabled_user off >password5 +@all +all %RW~* +&*",
					"user1 on +@all +all %RW~* +&*",
				},
			},
			{
				name: "3. Load users from the .yml file",
				path: path.Join(baseDir, "yml_test.yml"),
				users: []echovault.User{
					{Username: "user1", Enabled: true},
				},
				cmd: []resp.Value{resp.StringValue("ACL"), resp.StringValue("LOAD"), resp.StringValue("REPLACE")},
				want: []string{
					"default on +@all +all %RW~* +&*",
					fmt.Sprintf("with_password_user on >password2 #%s +@all +all %s~* +&*",
						generateSHA256Password("password3"), "%RW"),
					"no_password_user on nopass +@all +all %RW~* +&*",
					"disabled_user off >password5 +@all +all %RW~* +&*",
					"user1 on +@all +all %RW~* +&*",
				},
			},
			{
				name: "4. Merge loaded users",
				path: path.Join(baseDir, "merge.yml"),
				users: []echovault.User{
					{ // Disable user1.
						Username: "user1",
						Enabled:  false,
					},
					{ // Update with_password_user. This should be merged with the existing user.
						Username:             "with_password_user",
						AddPlainPasswords:    []string{"password3", "password4"},
						IncludeReadWriteKeys: []string{"key1", "key2"},
						IncludeWriteKeys:     []string{"key3", "key4"},
						IncludeReadKeys:      []string{"key5", "key6"},
						IncludeChannels:      []string{"channel[12]"},
						ExcludeChannels:      []string{"channel[34]"},
					},
				},
				cmd: []resp.Value{resp.StringValue("ACL"), resp.StringValue("LOAD"), resp.StringValue("MERGE")},
				want: []string{
					"default on +@all +all %RW~* +&*",
					fmt.Sprintf(`with_password_user on >password2 >password3 >password4 #%s +@all +all %s~key1 %s~key2 %s~key5 %s~key6 %s~key3 %s~key4 +&channel[12] -&channel[34]`,
						generateSHA256Password("password3"), "%RW", "%RW", "%R", "%R", "%W", "%W"),
					"no_password_user on nopass +@all +all %RW~* +&*",
					"disabled_user off >password5 +@all +all %RW~* +&*",
					"user1 off +@all +all %RW~* +&*",
				},
			},
			{
				name: "5. Replace loaded users",
				path: path.Join(baseDir, "replace.yml"),
				users: []echovault.User{
					{ // Disable user1.
						Username: "user1",
						Enabled:  false,
					},
					{ // Update with_password_user. This should be merged with the existing user.
						Username:             "with_password_user",
						AddPlainPasswords:    []string{"password3", "password4"},
						IncludeReadWriteKeys: []string{"key1", "key2"},
						IncludeWriteKeys:     []string{"key3", "key4"},
						IncludeReadKeys:      []string{"key5", "key6"},
						IncludeChannels:      []string{"channel[12]"},
						ExcludeChannels:      []string{"channel[34]"},
					},
				},
				cmd: []resp.Value{resp.StringValue("ACL"), resp.StringValue("LOAD"), resp.StringValue("REPLACE")},
				want: []string{
					"default on +@all +all %RW~* +&*",
					fmt.Sprintf("with_password_user on >password2 #%s +@all +all %s~* +&*",
						generateSHA256Password("password3"), "%RW"),
					"no_password_user on nopass +@all +all %RW~* +&*",
					"disabled_user off >password5 +@all +all %RW~* +&*",
					"user1 off +@all +all %RW~* +&*",
				},
			},
		}

		servers := make([]*echovault.EchoVault, len(tests))
		mut := sync.Mutex{}
		t.Cleanup(func() {
			_ = os.RemoveAll(baseDir)
			for _, server := range servers {
				if server != nil {
					server.ShutDown()
				}
			}
		})

		for i, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()
				mut.Lock()
				defer mut.Unlock()
				// Create server with pre-generated users.
				port, err := internal.GetFreePort()
				if err != nil {
					t.Error(err)
					return
				}
				mockServer, err := setUpServer(port, false, test.path)
				if err != nil {
					t.Error(err)
					return
				}
				servers[i] = mockServer
				go func() {
					mockServer.Start()
				}()

				// Save the current users to the ACL config file.
				if _, err := mockServer.ACLSave(); err != nil {
					t.Error(err)
					return
				}

				// Add some users to the ACL.
				for _, user := range test.users {
					if _, err := mockServer.ACLSetUser(user); err != nil {
						t.Error(err)
						return
					}
				}

				// Establish client connection
				conn, err := internal.GetConnection("localhost", port)
				if err != nil {
					t.Error(err)
					return
				}
				client := resp.NewConn(conn)

				// Load the users from the ACL config file.
				if err := client.WriteArray(test.cmd); err != nil {
					t.Error(err)
					return
				}

				res, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
					return
				}

				if !strings.EqualFold(res.String(), "ok") {
					t.Error(err)
					mockServer.ShutDown()
					return
				}

				// Get ACL List
				if err = client.WriteArray([]resp.Value{resp.StringValue("ACL"), resp.StringValue("LIST")}); err != nil {
					t.Error(err)
					return
				}

				res, _, err = client.ReadValue()
				if err != nil {
					t.Error(err)
					return
				}

				// Check if ACL LIST returns the expected list of users.
				resArr := res.Array()
				if len(resArr) != len(test.want) {
					t.Errorf("expected response of length %d, got lenght %d", len(test.want), len(resArr))
					return
				}

				var resStr []string
				for i := 0; i < len(resArr); i++ {
					resStr = strings.Split(resArr[i].String(), " ")
					if !slices.ContainsFunc(test.want, func(s string) bool {
						expectedUserSlice := strings.Split(s, " ")
						return compareSlices(resStr, expectedUserSlice) == nil
					}) {
						t.Errorf("could not find the following user in expected slice: %+v", resStr)
						return
					}
				}
			})
		}
	})
}
