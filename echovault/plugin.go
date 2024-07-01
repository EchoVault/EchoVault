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

package echovault

import (
	"context"
	"errors"
	"fmt"
	"github.com/echovault/echovault/internal"
	"io/fs"
	"os"
	"plugin"
	"slices"
	"strings"
)

// LoadModule loads an external module into EchoVault ar runtime.
//
// Parameters:
//
// `path` - string - The full path to the .so plugin to be loaded.
//
// `args` - ...string - A list of args that will be passed unmodified to the plugins command's
// KeyExtractionFunc and HandlerFunc
func (server *EchoVault) LoadModule(path string, args ...string) error {
	server.commandsRWMut.Lock()
	defer server.commandsRWMut.Unlock()

	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("load module: module %s not found", path)
		}
		return fmt.Errorf("load module: %v", err)
	}

	p, err := plugin.Open(path)
	if err != nil {
		return fmt.Errorf("plugin open: %v", err)
	}

	commandSymbol, err := p.Lookup("Command")
	if err != nil {
		return err
	}
	command, ok := commandSymbol.(*string)
	if !ok {
		return errors.New("command symbol is not a string")
	}

	categoriesSymbol, err := p.Lookup("Categories")
	if err != nil {
		return err
	}
	categories, ok := categoriesSymbol.(*[]string)
	if !ok {
		return errors.New("categories symbol not a string slice")
	}

	descriptionSymbol, err := p.Lookup("Description")
	if err != nil {
		return err
	}
	description, ok := descriptionSymbol.(*string)
	if !ok {
		return errors.New("description symbol is no a string")
	}

	syncSymbol, err := p.Lookup("Sync")
	if err != nil {
		return err
	}
	sync, ok := syncSymbol.(*bool)
	if !ok {
		return errors.New("sync symbol is not a bool")
	}

	keyExtractionFuncSymbol, err := p.Lookup("KeyExtractionFunc")
	if err != nil {
		return fmt.Errorf("key extraction func symbol: %v", err)
	}
	keyExtractionFunc, ok := keyExtractionFuncSymbol.(func(cmd []string, args ...string) ([]string, []string, error))
	if !ok {
		return errors.New("key extraction function has unexpected signature")
	}

	handlerFuncSymbol, err := p.Lookup("HandlerFunc")
	if err != nil {
		return fmt.Errorf("handler func symbol: %v", err)
	}
	handlerFunc, ok := handlerFuncSymbol.(func(
		ctx context.Context,
		command []string,
		keysExist func(ctx context.Context, key []string) map[string]bool,
		getValues func(ctx context.Context, key []string) map[string]interface{},
		setValues func(ctx context.Context, entries map[string]interface{}) error,
		args ...string,
	) ([]byte, error))
	if !ok {
		return errors.New("handler function has unexpected signature")
	}

	// Remove the currently loaded version of this module and replace it with the new one
	server.commands = slices.DeleteFunc(server.commands, func(command internal.Command) bool {
		return strings.EqualFold(command.Module, path)
	})

	// Add the new command
	server.commands = append(server.commands, internal.Command{
		Command: *command,
		Module:  path,
		Categories: func() []string {
			// Convert all the categories to lower case for uniformity
			cats := make([]string, len(*categories))
			for i, cat := range *categories {
				cats[i] = strings.ToLower(cat)
			}
			return cats
		}(),
		Description: *description,
		Sync:        *sync,
		SubCommands: make([]internal.SubCommand, 0),
		KeyExtractionFunc: func(cmd []string) (internal.KeyExtractionFuncResult, error) {
			readKeys, writeKeys, err := keyExtractionFunc(cmd, args...)
			if err != nil {
				return internal.KeyExtractionFuncResult{}, err
			}
			return internal.KeyExtractionFuncResult{
				Channels:  make([]string, 0),
				ReadKeys:  readKeys,
				WriteKeys: writeKeys,
			}, nil
		},
		HandlerFunc: func(params internal.HandlerFuncParams) ([]byte, error) {
			return handlerFunc(
				params.Context,
				params.Command,
				params.KeysExist,
				params.GetValues,
				params.SetValues,
				args...,
			)
		},
	})

	return nil
}

// UnloadModule unloads the provided module
//
// Parameters:
//
// `module` - string - module name as displayed by the ListModules method.
func (server *EchoVault) UnloadModule(module string) {
	server.commandsRWMut.Lock()
	defer server.commandsRWMut.Unlock()
	server.commands = slices.DeleteFunc(server.commands, func(command internal.Command) bool {
		return strings.EqualFold(command.Module, module)
	})
}

// ListModules lists the currently loaded modules
//
// Returns: a string slice representing all the currently loaded modules.
func (server *EchoVault) ListModules() []string {
	server.commandsRWMut.RLock()
	defer server.commandsRWMut.RUnlock()
	var modules []string
	for _, command := range server.commands {
		if !slices.ContainsFunc(modules, func(module string) bool {
			return strings.EqualFold(module, command.Module)
		}) {
			modules = append(modules, strings.ToLower(command.Module))
		}
	}
	return modules
}
