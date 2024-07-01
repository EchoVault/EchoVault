// Copyright 2024 Kelvin Clement Mwinuka
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package echovault

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/echovault/echovault/internal/constants"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
	"time"
)

func generateInitialTestUsers() []User {
	return []User{
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

func TestEchoVault_ACLCat(t *testing.T) {
	server := createEchoVault()

	getCategoryCommands := func(category string) []string {
		var commands []string
		for _, command := range server.commands {
			if slices.Contains(command.Categories, category) && (command.SubCommands == nil || len(command.SubCommands) == 0) {
				commands = append(commands, strings.ToLower(command.Command))
				continue
			}
			for _, subcommand := range command.SubCommands {
				if slices.Contains(subcommand.Categories, category) {
					commands = append(commands, strings.ToLower(fmt.Sprintf("%s|%s", command.Command, subcommand.Command)))
				}
			}
		}
		return commands
	}

	tests := []struct {
		name    string
		args    []string
		want    []string
		wantErr bool
	}{
		{
			name: "1. Get all ACL categories loaded on the server",
			args: make([]string, 0),
			want: []string{
				constants.AdminCategory, constants.ConnectionCategory, constants.DangerousCategory,
				constants.HashCategory, constants.FastCategory, constants.KeyspaceCategory, constants.ListCategory,
				constants.PubSubCategory, constants.ReadCategory, constants.WriteCategory, constants.SetCategory,
				constants.SortedSetCategory, constants.SlowCategory, constants.StringCategory,
			},
			wantErr: false,
		},
		{
			name:    "2. Get all commands within the admin category",
			args:    []string{constants.AdminCategory},
			want:    getCategoryCommands(constants.AdminCategory),
			wantErr: false,
		},
		{
			name:    "3. Get all commands within the connection category",
			args:    []string{constants.ConnectionCategory},
			want:    getCategoryCommands(constants.ConnectionCategory),
			wantErr: false,
		},
		{
			name:    "4. Get all the commands within the dangerous category",
			args:    []string{constants.DangerousCategory},
			want:    getCategoryCommands(constants.DangerousCategory),
			wantErr: false,
		},
		{
			name:    "5. Get all the commands within the hash category",
			args:    []string{constants.HashCategory},
			want:    getCategoryCommands(constants.HashCategory),
			wantErr: false,
		},
		{
			name:    "6. Get all the commands within the fast category",
			args:    []string{constants.FastCategory},
			want:    getCategoryCommands(constants.FastCategory),
			wantErr: false,
		},
		{
			name:    "7. Get all the commands within the keyspace category",
			args:    []string{constants.KeyspaceCategory},
			want:    getCategoryCommands(constants.KeyspaceCategory),
			wantErr: false,
		},
		{
			name:    "8. Get all the commands within the list category",
			args:    []string{constants.ListCategory},
			want:    getCategoryCommands(constants.ListCategory),
			wantErr: false,
		},
		{
			name:    "9. Get all the commands within the pubsub category",
			args:    []string{constants.PubSubCategory},
			want:    getCategoryCommands(constants.PubSubCategory),
			wantErr: false,
		},
		{
			name:    "10. Get all the commands within the read category",
			args:    []string{constants.ReadCategory},
			want:    getCategoryCommands(constants.ReadCategory),
			wantErr: false,
		},
		{
			name:    "11. Get all the commands within the write category",
			args:    []string{constants.WriteCategory},
			want:    getCategoryCommands(constants.WriteCategory),
			wantErr: false,
		},
		{
			name:    "12. Get all the commands within the set category",
			args:    []string{constants.SetCategory},
			want:    getCategoryCommands(constants.SetCategory),
			wantErr: false,
		},
		{
			name:    "13. Get all the commands within the sortedset category",
			args:    []string{constants.SortedSetCategory},
			want:    getCategoryCommands(constants.SortedSetCategory),
			wantErr: false,
		},
		{
			name:    "14. Get all the commands within the slow category",
			args:    []string{constants.SlowCategory},
			want:    getCategoryCommands(constants.SlowCategory),
			wantErr: false,
		},
		{
			name:    "15. Get all the commands within the string category",
			args:    []string{constants.StringCategory},
			want:    getCategoryCommands(constants.StringCategory),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := server.ACLCat(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Errorf("ACLCat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("ACLCat() got length = %d, want length %d", len(got), len(tt.want))
			}
			for _, item := range got {
				if !slices.Contains(tt.want, item) {
					t.Errorf("ACLCat() got unexpected element = %s, want %v", item, tt.want)
				}
			}
		})
	}
}

func TestEchoVault_ACLUsers(t *testing.T) {
	server := createEchoVault()

	// Set Users
	users := []User{
		{
			Username:             "user1",
			Enabled:              true,
			NoPassword:           true,
			NoKeys:               true,
			NoCommands:           true,
			AddPlainPasswords:    []string{},
			AddHashPasswords:     []string{},
			IncludeCategories:    []string{},
			IncludeReadWriteKeys: []string{},
			IncludeReadKeys:      []string{},
			IncludeWriteKeys:     []string{},
			IncludeChannels:      []string{},
			ExcludeChannels:      []string{},
		},
		{
			Username:          "user2",
			Enabled:           true,
			NoPassword:        false,
			NoKeys:            false,
			NoCommands:        false,
			AddPlainPasswords: []string{"password1", "password2"},
			AddHashPasswords: []string{
				func() string {
					h := sha256.New()
					h.Write([]byte("password1"))
					return string(h.Sum(nil))
				}(),
			},
			IncludeCategories:    []string{constants.FastCategory, constants.SlowCategory, constants.HashCategory},
			ExcludeCategories:    []string{constants.AdminCategory, constants.DangerousCategory},
			IncludeCommands:      []string{"*"},
			ExcludeCommands:      []string{"acl|load", "acl|save"},
			IncludeReadWriteKeys: []string{"user2-profile-*"},
			IncludeReadKeys:      []string{"user2-privileges-*"},
			IncludeWriteKeys:     []string{"write-key"},
			IncludeChannels:      []string{"posts-*"},
			ExcludeChannels:      []string{"actions-*"},
		},
	}

	for _, user := range users {
		ok, err := server.ACLSetUser(user)
		if err != nil {
			t.Errorf("ACLSetUser() err = %v", err)
		}
		if !ok {
			t.Errorf("ACLSetUser() ok = %v", ok)
		}
	}

	// Get users
	aclUsers, err := server.ACLUsers()
	if err != nil {
		t.Errorf("ACLUsers() err = %v", err)
	}
	if len(aclUsers) != len(users)+1 {
		t.Errorf("ACLUsers() got length %d, want %d", len(aclUsers), len(users)+1)
	}
	for _, username := range aclUsers {
		if !slices.Contains([]string{"default", "user1", "user2"}, username) {
			t.Errorf("ACLUsers() unexpected username = %s", username)
		}
	}

	// Get specific user.
	user, err := server.ACLGetUser("user2")
	if err != nil {
		t.Errorf("ACLGetUser() err = %v", err)
	}
	if user == nil {
		t.Errorf("ACLGetUser() user is nil")
	}

	// Delete user
	ok, err := server.ACLDelUser("user1")
	if err != nil {
		t.Errorf("ACLDelUser() err = %v", err)
	}
	if !ok {
		t.Errorf("ACLDelUser() could not delete user user1")
	}
	aclUsers, err = server.ACLUsers()
	if err != nil {
		t.Errorf("ACLDelUser() err = %v", err)
	}
	if slices.Contains(aclUsers, "user1") {
		t.Errorf("ACLDelUser() unexpected username user1")
	}

	// Get list of currently loaded ACL rules.
	list, err := server.ACLList()
	if err != nil {
		t.Errorf("ACLList() err = %v", err)
	}
	if len(list) != 2 {
		t.Errorf("ACLList() got list length %d, want %d", len(list), 2)
	}
}

func TestEchoVault_ACLConfig(t *testing.T) {
	t.Run("Test_HandleSave", func(t *testing.T) {
		baseDir := path.Join(".", "testdata", "save")
		t.Cleanup(func() {
			_ = os.RemoveAll(baseDir)
		})

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

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				// Create new server instance
				conf := DefaultConfig()
				conf.DataDir = ""
				conf.AclConfig = test.path
				server := createEchoVaultWithConfig(conf)
				// Add the initial test users to the ACL module.
				for _, user := range generateInitialTestUsers() {
					if _, err := server.ACLSetUser(user); err != nil {
						t.Error(err)
						return
					}
				}

				ok, err := server.ACLSave()
				if err != nil {
					t.Error(err)
					return
				}
				if !ok {
					t.Errorf("expected ok to be true, got false")
				}

				// Shutdown the mock server
				server.ShutDown()

				// Restart server
				server = createEchoVaultWithConfig(conf)

				// Get users rules list.
				list, err := server.ACLList()

				// Check if ACL LIST returns the expected list of users.
				var resStr []string
				for i := 0; i < len(list); i++ {
					resStr = strings.Split(list[i], " ")
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
		baseDir := path.Join(".", "testdata", "load")
		t.Cleanup(func() {
			_ = os.RemoveAll(baseDir)
		})

		tests := []struct {
			name     string
			path     string
			users    []User                                // Add users after server startup.
			loadFunc func(server *EchoVault) (bool, error) // Function to load users from ACL config.
			want     []string
		}{
			{
				name: "1. Load config from the .json file",
				path: path.Join(baseDir, "json_test.json"),
				users: []User{
					{Username: "user1", Enabled: true},
				},
				loadFunc: func(server *EchoVault) (bool, error) {
					return server.ACLLoad(ACLLoadOptions{})
				},
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
				users: []User{
					{Username: "user1", Enabled: true},
				},
				loadFunc: func(server *EchoVault) (bool, error) {
					return server.ACLLoad(ACLLoadOptions{})
				},
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
				users: []User{
					{Username: "user1", Enabled: true},
				},
				loadFunc: func(server *EchoVault) (bool, error) {
					return server.ACLLoad(ACLLoadOptions{})
				},
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
				users: []User{
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
				loadFunc: func(server *EchoVault) (bool, error) {
					return server.ACLLoad(ACLLoadOptions{Merge: true, Replace: false})
				},
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
				users: []User{
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
				loadFunc: func(server *EchoVault) (bool, error) {
					return server.ACLLoad(ACLLoadOptions{Replace: true, Merge: false})
				},
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

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				// Create server.
				conf := DefaultConfig()
				conf.DataDir = ""
				conf.AclConfig = test.path
				server := createEchoVaultWithConfig(conf)
				// Add the initial test users to the ACL module.
				for _, user := range generateInitialTestUsers() {
					if _, err := server.ACLSetUser(user); err != nil {
						t.Error(err)
						return
					}
				}

				// Save the current users to the ACL config file.
				if _, err := server.ACLSave(); err != nil {
					t.Error(err)
					return
				}

				ticker := time.NewTicker(200 * time.Millisecond)
				<-ticker.C

				// Add some users to the ACL.
				for _, user := range test.users {
					if _, err := server.ACLSetUser(user); err != nil {
						t.Error(err)
						return
					}
				}

				// Load the users from the ACL config file.
				ok, err := test.loadFunc(server)
				if err != nil {
					t.Error(err)
					return
				}
				if !ok {
					t.Errorf("expected ok to be true, got false")
					return
				}

				// Get ACL List
				list, err := server.ACLList()
				if err != nil {
					t.Error(err)
					return
				}

				// Check if ACL LIST returns the expected list of users.
				var resStr []string
				for i := 0; i < len(list); i++ {
					resStr = strings.Split(list[i], " ")
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
