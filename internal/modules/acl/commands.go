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

package acl

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/echovault/echovault/internal"
	"github.com/echovault/echovault/internal/constants"
	"gopkg.in/yaml.v3"
	"log"
	"os"
	"path"
	"slices"
	"strings"
)

func handleCat(params internal.HandlerFuncParams) ([]byte, error) {
	if len(params.Command) > 3 {
		return nil, errors.New(constants.WrongArgsResponse)
	}

	categories := make(map[string][]string)

	commands := params.GetAllCommands()

	for _, command := range commands {
		if len(command.SubCommands) == 0 {
			for _, category := range command.Categories {
				categories[category] = append(categories[category], command.Command)
			}
			continue
		}
		for _, subcommand := range command.SubCommands {
			for _, category := range subcommand.Categories {
				categories[category] = append(categories[category],
					fmt.Sprintf("%s|%s", command.Command, subcommand.Command))
			}
		}
	}

	if len(params.Command) == 2 {
		var cats []string
		length := 0
		for key, _ := range categories {
			cats = append(cats, key)
			length += 1
		}
		res := fmt.Sprintf("*%d", length)
		for i, cat := range cats {
			res = fmt.Sprintf("%s\r\n+%s", res, cat)
			if i == len(cats)-1 {
				res = res + "\r\n"
			}
		}
		return []byte(res), nil
	}

	if len(params.Command) == 3 {
		var res string
		for category, commands := range categories {
			if strings.EqualFold(category, params.Command[2]) {
				res = fmt.Sprintf("*%d", len(commands))
				for i, command := range commands {
					res = fmt.Sprintf("%s\r\n+%s", res, command)
					if i == len(commands)-1 {
						res = res + "\r\n"
					}
				}
				return []byte(res), nil
			}
		}
	}

	return nil, fmt.Errorf("category %s not found", strings.ToUpper(params.Command[2]))
}

func handleGetUser(params internal.HandlerFuncParams) ([]byte, error) {
	if len(params.Command) != 3 {
		return nil, errors.New(constants.WrongArgsResponse)
	}

	acl, ok := params.GetACL().(*ACL)
	if !ok {
		return nil, errors.New("could not load ACL")
	}
	acl.RLockUsers()
	defer acl.RUnlockUsers()

	var user *User
	userFound := false
	for _, u := range acl.Users {
		if u.Username == params.Command[2] {
			user = u
			userFound = true
			break
		}
	}

	if !userFound {
		return nil, errors.New("user not found")
	}

	// username,
	res := fmt.Sprintf("*12\r\n+username\r\n*1\r\n+%s", user.Username)

	// flags
	var flags []string
	if user.Enabled {
		flags = append(flags, "on")
	} else {
		flags = append(flags, "off")
	}
	if user.NoPassword {
		flags = append(flags, "nopass")
	}
	if user.NoKeys {
		flags = append(flags, "nokeys")
	}

	res = res + fmt.Sprintf("\r\n+flags\r\n*%d", len(flags))
	for _, flag := range flags {
		res = fmt.Sprintf("%s\r\n+%s", res, flag)
	}

	// categories
	res = res + fmt.Sprintf("\r\n+categories\r\n*%d", len(user.IncludedCategories)+len(user.ExcludedCategories))
	for _, category := range user.IncludedCategories {
		if category == "*" {
			res = res + fmt.Sprintf("\r\n++@all")
			continue
		}
		res = res + fmt.Sprintf("\r\n++@%s", category)
	}
	for _, category := range user.ExcludedCategories {
		if category == "*" {
			res = res + fmt.Sprintf("\r\n+-@all")
			continue
		}
		res = res + fmt.Sprintf("\r\n+-@%s", category)
	}

	// commands
	res = res + fmt.Sprintf("\r\n+commands\r\n*%d", len(user.IncludedCommands)+len(user.ExcludedCommands))
	for _, command := range user.IncludedCommands {
		if command == "*" {
			res = res + fmt.Sprintf("\r\n++all")
			continue
		}
		res = res + fmt.Sprintf("\r\n++%s", command)
	}
	for _, command := range user.ExcludedCommands {
		if command == "*" {
			res = res + fmt.Sprintf("\r\n+-all")
			continue
		}
		res = res + fmt.Sprintf("\r\n+-%s", command)
	}

	// keys
	allKeys := user.IncludedReadKeys
	for _, key := range append(user.IncludedWriteKeys, user.IncludedReadKeys...) {
		if !slices.Contains(allKeys, key) {
			allKeys = append(allKeys, key)
		}
	}
	res = res + fmt.Sprintf("\r\n+keys\r\n*%d", len(allKeys))
	for _, key := range allKeys {
		switch {
		case slices.Contains(user.IncludedWriteKeys, key) && slices.Contains(user.IncludedReadKeys, key):
			// Key is RW
			res = res + fmt.Sprintf("\r\n+%s~%s", "%RW", key)
		case slices.Contains(user.IncludedWriteKeys, key):
			// Keys is W-Only
			res = res + fmt.Sprintf("\r\n+%s~%s", "%W", key)
		case slices.Contains(user.IncludedReadKeys, key):
			// Key is R-Only
			res = res + fmt.Sprintf("\r\n+%s~%s", "%R", key)
		}
	}

	// channels
	res = res + fmt.Sprintf("\r\n+channels\r\n*%d",
		len(user.IncludedPubSubChannels)+len(user.ExcludedPubSubChannels))
	for _, channel := range user.IncludedPubSubChannels {
		res = res + fmt.Sprintf("\r\n++&%s", channel)
	}
	for _, channel := range user.ExcludedPubSubChannels {
		res = res + fmt.Sprintf("\r\n+-&%s", channel)
	}

	res += "\r\n"

	return []byte(res), nil
}

func handleUsers(params internal.HandlerFuncParams) ([]byte, error) {
	acl, ok := params.GetACL().(*ACL)
	if !ok {
		return nil, errors.New("could not load ACL")
	}

	res := fmt.Sprintf("*%d", len(acl.Users))
	for _, user := range acl.Users {
		res += fmt.Sprintf("\r\n$%d\r\n%s", len(user.Username), user.Username)
	}
	res += "\r\n"
	return []byte(res), nil
}

func handleSetUser(params internal.HandlerFuncParams) ([]byte, error) {
	acl, ok := params.GetACL().(*ACL)
	if !ok {
		return nil, errors.New("could not load ACL")
	}
	if err := acl.SetUser(params.Command[2:]); err != nil {
		return nil, err
	}
	return []byte(constants.OkResponse), nil
}

func handleDelUser(params internal.HandlerFuncParams) ([]byte, error) {
	if len(params.Command) < 3 {
		return nil, errors.New(constants.WrongArgsResponse)
	}
	acl, ok := params.GetACL().(*ACL)
	if !ok {
		return nil, errors.New("could not load ACL")
	}
	if err := acl.DeleteUser(params.Context, params.Command[2:]); err != nil {
		return nil, err
	}
	return []byte(constants.OkResponse), nil
}

func handleWhoAmI(params internal.HandlerFuncParams) ([]byte, error) {
	acl, ok := params.GetACL().(*ACL)
	if !ok {
		return nil, errors.New("could not load ACL")
	}
	acl.RLockUsers()
	defer acl.RUnlockUsers()

	connectionInfo := acl.Connections[params.Connection]
	return []byte(fmt.Sprintf("+%s\r\n", connectionInfo.User.Username)), nil
}

func handleList(params internal.HandlerFuncParams) ([]byte, error) {
	if len(params.Command) > 2 {
		return nil, errors.New(constants.WrongArgsResponse)
	}
	acl, ok := params.GetACL().(*ACL)
	if !ok {
		return nil, errors.New("could not load ACL")
	}
	acl.RLockUsers()
	defer acl.RUnlockUsers()

	res := fmt.Sprintf("*%d", len(acl.Users))
	s := ""
	for _, user := range acl.Users {
		s = user.Username
		// User enabled
		if user.Enabled {
			s += " on"
		} else {
			s += " off"
		}
		// NoPassword
		if user.NoPassword {
			s += " nopass"
		}
		// No keys
		if user.NoKeys {
			s += " nokeys"
		}
		// Passwords
		for _, password := range user.Passwords {
			if strings.EqualFold(password.PasswordType, "plaintext") {
				s += fmt.Sprintf(" >%s", password.PasswordValue)
			}
			if strings.EqualFold(password.PasswordType, "SHA256") {
				s += fmt.Sprintf(" #%s", password.PasswordValue)
			}
		}
		// Included categories
		for _, category := range user.IncludedCategories {
			if category == "*" {
				s += " +@all"
				continue
			}
			s += fmt.Sprintf(" +@%s", category)
		}
		// Excluded categories
		for _, category := range user.ExcludedCategories {
			if category == "*" {
				s += " -@all"
				continue
			}
			s += fmt.Sprintf(" -@%s", category)
		}
		// Included commands
		for _, command := range user.IncludedCommands {
			if command == "*" {
				s += " +all"
				continue
			}
			s += fmt.Sprintf(" +%s", command)
		}
		// Excluded commands
		for _, command := range user.ExcludedCommands {
			if command == "*" {
				s += " -all"
				continue
			}
			s += fmt.Sprintf(" -%s", command)
		}
		// Included read keys
		for _, key := range user.IncludedReadKeys {
			if slices.Contains(user.IncludedWriteKeys, key) {
				s += fmt.Sprintf(" %s~%s", "%RW", key)
				continue
			}
			s += fmt.Sprintf(" %s~%s", "%R", key)
		}
		// Included write keys
		for _, key := range user.IncludedWriteKeys {
			if !slices.Contains(user.IncludedReadKeys, key) {
				s += fmt.Sprintf(" %s~%s", "%W", key)
			}
		}
		// Included Pub/Sub channels
		for _, channel := range user.IncludedPubSubChannels {
			s += fmt.Sprintf(" +&%s", channel)
		}
		// Excluded Pup/Sub channels
		for _, channel := range user.ExcludedPubSubChannels {
			s += fmt.Sprintf(" -&%s", channel)
		}
		res = res + fmt.Sprintf("\r\n$%d\r\n%s", len(s), s)
	}

	res = res + "\r\n"
	return []byte(res), nil
}

func handleLoad(params internal.HandlerFuncParams) ([]byte, error) {
	if len(params.Command) != 3 {
		return nil, errors.New(constants.WrongArgsResponse)
	}

	acl, ok := params.GetACL().(*ACL)
	if !ok {
		return nil, errors.New("could not load ACL")
	}
	acl.LockUsers()
	defer acl.UnlockUsers()

	f, err := os.OpenFile(acl.Config.AclConfig, os.O_RDONLY, os.ModePerm)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := f.Close(); err != nil {
			log.Println(err)
		}
	}()

	ext := path.Ext(f.Name())

	var users []*User

	if strings.ToLower(ext) == ".json" {
		if err := json.NewDecoder(f).Decode(&users); err != nil {
			return nil, err
		}
	}

	if slices.Contains([]string{".yaml", ".yml"}, strings.ToLower(ext)) {
		if err := yaml.NewDecoder(f).Decode(&users); err != nil {
			return nil, err
		}
	}

	// Normalise each user
	for _, user := range users {
		user.Normalise()
		// Traverse the list of users.
		userFound := false
		for _, u := range acl.Users {
			if u.Username == user.Username {
				userFound = true
				// If we have a user with the current username and are in merge mode, merge the two users.
				if strings.EqualFold(params.Command[2], "merge") {
					u.Merge(user)
				} else {
					// If we have a user with the current username and are in replace mode, merge the two users.
					u.Replace(user)
				}
				break
			}
		}
		// If there is no user with current loaded username is already in acl list, then append the user to the list
		if !userFound {
			acl.Users = append(acl.Users, user)
		}
	}

	return []byte(constants.OkResponse), nil
}

func handleSave(params internal.HandlerFuncParams) ([]byte, error) {
	if len(params.Command) > 2 {
		return nil, errors.New(constants.WrongArgsResponse)
	}

	acl, ok := params.GetACL().(*ACL)
	if !ok {
		return nil, errors.New("could not load ACL")
	}
	acl.RLockUsers()
	defer acl.RUnlockUsers()

	f, err := os.OpenFile(acl.Config.AclConfig, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := f.Close(); err != nil {
			log.Println(err)
		}
	}()

	ext := path.Ext(f.Name())

	if strings.ToLower(ext) == ".json" {
		// Write to JSON config file
		out, err := json.Marshal(acl.Users)
		if err != nil {
			return nil, err
		}
		if _, err = f.Write(out); err != nil {
			return nil, err
		}
	}

	if slices.Contains([]string{".yaml", ".yml"}, strings.ToLower(ext)) {
		// Write to yaml file
		out, err := yaml.Marshal(acl.Users)
		if err != nil {
			return nil, err
		}
		if _, err = f.Write(out); err != nil {
			return nil, err
		}
	}

	if err = f.Sync(); err != nil {
		return nil, err
	}

	return []byte(constants.OkResponse), nil
}

func Commands() []internal.Command {
	return []internal.Command{
		{
			Command:     "acl",
			Module:      constants.ACLModule,
			Categories:  []string{},
			Description: "Access-Control-List commands",
			Sync:        false,
			KeyExtractionFunc: func(cmd []string) (internal.KeyExtractionFuncResult, error) {
				return internal.KeyExtractionFuncResult{
					Channels:  make([]string, 0),
					ReadKeys:  make([]string, 0),
					WriteKeys: make([]string, 0),
				}, nil
			},
			SubCommands: []internal.SubCommand{
				{
					Command:    "cat",
					Module:     constants.ACLModule,
					Categories: []string{constants.SlowCategory},
					Description: `(ACL CAT [category]) Lists all the categories. 
If the optional category is provided, lists all the commands in the category.`,
					Sync: false,
					KeyExtractionFunc: func(cmd []string) (internal.KeyExtractionFuncResult, error) {
						return internal.KeyExtractionFuncResult{
							Channels:  make([]string, 0),
							ReadKeys:  make([]string, 0),
							WriteKeys: make([]string, 0),
						}, nil
					},
					HandlerFunc: handleCat,
				},
				{
					Command:     "users",
					Module:      constants.ACLModule,
					Categories:  []string{constants.AdminCategory, constants.SlowCategory, constants.DangerousCategory},
					Description: "(ACL USERS) Lists all usernames of the configured ACL users.",
					Sync:        false,
					KeyExtractionFunc: func(cmd []string) (internal.KeyExtractionFuncResult, error) {
						return internal.KeyExtractionFuncResult{
							Channels:  make([]string, 0),
							ReadKeys:  make([]string, 0),
							WriteKeys: make([]string, 0),
						}, nil
					},
					HandlerFunc: handleUsers,
				},
				{
					Command:     "setuser",
					Module:      constants.ACLModule,
					Categories:  []string{constants.AdminCategory, constants.SlowCategory, constants.DangerousCategory},
					Description: "(ACL SETUSER) Configure a new or existing user",
					Sync:        true,
					KeyExtractionFunc: func(cmd []string) (internal.KeyExtractionFuncResult, error) {
						return internal.KeyExtractionFuncResult{
							Channels:  make([]string, 0),
							ReadKeys:  make([]string, 0),
							WriteKeys: make([]string, 0),
						}, nil
					},
					HandlerFunc: handleSetUser,
				},
				{
					Command:     "getuser",
					Module:      constants.ACLModule,
					Categories:  []string{constants.AdminCategory, constants.SlowCategory, constants.DangerousCategory},
					Description: "(ACL GETUSER username) List the ACL rules of a user.",
					Sync:        false,
					KeyExtractionFunc: func(cmd []string) (internal.KeyExtractionFuncResult, error) {
						return internal.KeyExtractionFuncResult{
							Channels:  make([]string, 0),
							ReadKeys:  make([]string, 0),
							WriteKeys: make([]string, 0),
						}, nil
					},
					HandlerFunc: handleGetUser,
				},
				{
					Command:    "deluser",
					Module:     constants.ACLModule,
					Categories: []string{constants.AdminCategory, constants.SlowCategory, constants.DangerousCategory},
					Description: `(ACL DELUSER username [username ...]) 
Deletes users and terminates their connections. Cannot delete default user.`,
					Sync: true,
					KeyExtractionFunc: func(cmd []string) (internal.KeyExtractionFuncResult, error) {
						return internal.KeyExtractionFuncResult{
							Channels:  make([]string, 0),
							ReadKeys:  make([]string, 0),
							WriteKeys: make([]string, 0),
						}, nil
					},
					HandlerFunc: handleDelUser,
				},
				{
					Command:     "whoami",
					Module:      constants.ACLModule,
					Categories:  []string{constants.FastCategory},
					Description: "(ACL WHOAMI) Returns the authenticated user of the current connection.",
					Sync:        true,
					KeyExtractionFunc: func(cmd []string) (internal.KeyExtractionFuncResult, error) {
						return internal.KeyExtractionFuncResult{
							Channels:  make([]string, 0),
							ReadKeys:  make([]string, 0),
							WriteKeys: make([]string, 0),
						}, nil
					},
					HandlerFunc: handleWhoAmI,
				},
				{
					Command:     "list",
					Module:      constants.ACLModule,
					Categories:  []string{constants.AdminCategory, constants.SlowCategory, constants.DangerousCategory},
					Description: "(ACL LIST) Dumps effective acl rules in ACL DSL format.",
					Sync:        true,
					KeyExtractionFunc: func(cmd []string) (internal.KeyExtractionFuncResult, error) {
						return internal.KeyExtractionFuncResult{
							Channels:  make([]string, 0),
							ReadKeys:  make([]string, 0),
							WriteKeys: make([]string, 0),
						}, nil
					},
					HandlerFunc: handleList,
				},
				{
					Command:    "load",
					Module:     constants.ACLModule,
					Categories: []string{constants.AdminCategory, constants.SlowCategory, constants.DangerousCategory},
					Description: `
(ACL LOAD <MERGE | REPLACE>) Reloads the rules from the configured ACL config file.
When 'MERGE' is passed, users from config file who share a username with users in memory will be merged.
When 'REPLACE' is passed, users from config file who share a username with users in memory will replace the user in memory.`,
					Sync: true,
					KeyExtractionFunc: func(cmd []string) (internal.KeyExtractionFuncResult, error) {
						return internal.KeyExtractionFuncResult{
							Channels:  make([]string, 0),
							ReadKeys:  make([]string, 0),
							WriteKeys: make([]string, 0),
						}, nil
					},
					HandlerFunc: handleLoad,
				},
				{
					Command:     "save",
					Module:      constants.ACLModule,
					Categories:  []string{constants.AdminCategory, constants.SlowCategory, constants.DangerousCategory},
					Description: "(ACL SAVE) Saves the effective ACL rules the configured ACL config file.",
					Sync:        true,
					KeyExtractionFunc: func(cmd []string) (internal.KeyExtractionFuncResult, error) {
						return internal.KeyExtractionFuncResult{
							Channels:  make([]string, 0),
							ReadKeys:  make([]string, 0),
							WriteKeys: make([]string, 0),
						}, nil
					},
					HandlerFunc: handleSave,
				},
			},
		},
	}
}
