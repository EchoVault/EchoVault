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

package list

import (
	"errors"
	"fmt"
	"github.com/echovault/echovault/internal"
	"github.com/echovault/echovault/internal/constants"
	"math"
	"slices"
	"strings"
)

func handleLLen(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := llenKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	keyExists := params.KeysExist(params.Context, keys.ReadKeys)[key]

	if !keyExists {
		// If key does not exist, return 0
		return []byte(":0\r\n"), nil
	}

	if list, ok := params.GetValues(params.Context, []string{key})[key].([]interface{}); ok {
		return []byte(fmt.Sprintf(":%d\r\n", len(list))), nil
	}

	return nil, errors.New("LLEN command on non-list item")
}

func handleLIndex(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := lindexKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	keyExists := params.KeysExist(params.Context, keys.ReadKeys)[key]
	index, ok := internal.AdaptType(params.Command[2]).(int)

	if !ok {
		return nil, errors.New("index must be an integer")
	}

	if !keyExists {
		return nil, errors.New("LINDEX command on non-list item")
	}

	list, ok := params.GetValues(params.Context, []string{key})[key].([]interface{})
	if !ok {
		return nil, errors.New("LINDEX command on non-list item")
	}

	if !(index >= 0 && index < len(list)) {
		return nil, errors.New("index must be within list range")
	}

	return []byte(fmt.Sprintf("+%s\r\n", list[index])), nil
}

func handleLRange(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := lrangeKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	keyExists := params.KeysExist(params.Context, keys.ReadKeys)[key]
	start, startOk := internal.AdaptType(params.Command[2]).(int)
	end, endOk := internal.AdaptType(params.Command[3]).(int)

	if !startOk || !endOk {
		return nil, errors.New("start and end indices must be integers")
	}

	if !keyExists {
		return nil, errors.New("LRANGE command on non-list item")
	}

	list, ok := params.GetValues(params.Context, []string{key})[key].([]interface{})
	if !ok {
		return nil, errors.New("LRANGE command on non-list item")
	}

	// Make sure start is within range
	if !(start >= 0 && start < len(list)) {
		return nil, errors.New("start index must be within list boundary")
	}

	// Make sure end is within range, or is -1 otherwise
	if !((end >= 0 && end < len(list)) || end == -1) {
		return nil, errors.New("end index must be within list range or -1")
	}

	var bytes []byte

	// If end is -1, read list from start to the end of the list
	if end == -1 {
		bytes = []byte("*" + fmt.Sprint(len(list)-int(start)) + "\r\n")
		for i := int(start); i < len(list); i++ {
			str := fmt.Sprintf("%v", list[i])
			bytes = append(bytes, []byte("$"+fmt.Sprint(len(str))+"\r\n"+str+"\r\n")...)
		}
		return bytes, nil
	}

	// Make sure start and end are not equal to each other
	if start == end {
		return nil, errors.New("start and end indices cannot be equal")
	}

	// If end is not -1:
	//	1) If end is larger than start, return slice from start -> end
	//	2) If end is smaller than start, return slice from end -> start
	bytes = []byte("*" + fmt.Sprint(int(math.Abs(float64(start-end)))+1) + "\r\n")

	i := start
	j := end + 1
	if start > end {
		j = end - 1
	}

	for i != j {
		str := fmt.Sprintf("%v", list[i])
		bytes = append(bytes, []byte("$"+fmt.Sprint(len(str))+"\r\n"+str+"\r\n")...)
		if start < end {
			i++
		} else {
			i--
		}
	}

	return bytes, nil
}

func handleLSet(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := lsetKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	keyExists := params.KeysExist(params.Context, keys.WriteKeys)[key]

	index, ok := internal.AdaptType(params.Command[2]).(int)
	if !ok {
		return nil, errors.New("index must be an integer")
	}

	if !keyExists {
		return nil, errors.New("LSET command on non-list item")
	}

	list, ok := params.GetValues(params.Context, []string{key})[key].([]interface{})
	if !ok {
		return nil, errors.New("LSET command on non-list item")
	}

	if !(index >= 0 && index < len(list)) {
		return nil, errors.New("index must be within list range")
	}

	list[index] = internal.AdaptType(params.Command[3])
	if err = params.SetValues(params.Context, map[string]interface{}{key: list}); err != nil {
		return nil, err
	}

	return []byte(constants.OkResponse), nil
}

func handleLTrim(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := ltrimKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	keyExists := params.KeysExist(params.Context, keys.WriteKeys)[key]
	start, startOk := internal.AdaptType(params.Command[2]).(int)
	end, endOk := internal.AdaptType(params.Command[3]).(int)

	if !startOk || !endOk {
		return nil, errors.New("start and end indices must be integers")
	}

	if end < start && end != -1 {
		return nil, errors.New("end index must be greater than start index or -1")
	}

	if !keyExists {
		return nil, errors.New("LTRIM command on non-list item")
	}

	list, ok := params.GetValues(params.Context, []string{key})[key].([]interface{})
	if !ok {
		return nil, errors.New("LTRIM command on non-list item")
	}

	if !(start >= 0 && start < len(list)) {
		return nil, errors.New("start index must be within list boundary")
	}

	if end == -1 || end > len(list) {
		if err = params.SetValues(params.Context, map[string]interface{}{key: list[start:]}); err != nil {
			return nil, err
		}
		return []byte(constants.OkResponse), nil
	}

	if err = params.SetValues(params.Context, map[string]interface{}{key: list[start:end]}); err != nil {
		return nil, err
	}
	return []byte(constants.OkResponse), nil
}

func handleLRem(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := lremKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	keyExists := params.KeysExist(params.Context, keys.WriteKeys)[key]
	value := params.Command[3]

	count, ok := internal.AdaptType(params.Command[2]).(int)
	if !ok {
		return nil, errors.New("count must be an integer")
	}

	absoluteCount := internal.AbsInt(count)

	if !keyExists {
		return nil, errors.New("LREM command on non-list item")
	}

	list, ok := params.GetValues(params.Context, []string{key})[key].([]interface{})
	if !ok {
		return nil, errors.New("LREM command on non-list item")
	}

	switch {
	default:
		// Count is zero, keep list the same
	case count > 0:
		// Start from the head
		for i := 0; i < len(list); i++ {
			if absoluteCount == 0 {
				break
			}
			if fmt.Sprintf("%v", list[i]) == value {
				list[i] = nil
				absoluteCount -= 1
			}
		}
	case count < 0:
		// Start from the tail
		for i := len(list) - 1; i >= 0; i-- {
			if absoluteCount == 0 {
				break
			}
			if fmt.Sprintf("%v", list[i]) == value {
				list[i] = nil
				absoluteCount -= 1
			}
		}
	}

	list = slices.DeleteFunc(list, func(elem interface{}) bool {
		return elem == nil
	})

	if err = params.SetValues(params.Context, map[string]interface{}{key: list}); err != nil {
		return nil, err
	}

	return []byte(constants.OkResponse), nil
}

func handleLMove(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := lmoveKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	keysExist := params.KeysExist(params.Context, keys.WriteKeys)
	source, destination := keys.WriteKeys[0], keys.WriteKeys[1]
	whereFrom := strings.ToLower(params.Command[3])
	whereTo := strings.ToLower(params.Command[4])

	if !slices.Contains([]string{"left", "right"}, whereFrom) || !slices.Contains([]string{"left", "right"}, whereTo) {
		return nil, errors.New("wherefrom and whereto arguments must be either LEFT or RIGHT")
	}

	if !keysExist[source] || !keysExist[destination] {
		return nil, errors.New("both source and destination must be lists")
	}

	lists := params.GetValues(params.Context, keys.WriteKeys)
	sourceList, sourceOk := lists[source].([]interface{})
	destinationList, destinationOk := lists[destination].([]interface{})

	if !sourceOk || !destinationOk {
		return nil, errors.New("both source and destination must be lists")
	}

	switch whereFrom {
	case "left":
		err = params.SetValues(params.Context, map[string]interface{}{
			source: append([]interface{}{}, sourceList[1:]...),
			destination: func() []interface{} {
				if whereTo == "left" {
					return append(sourceList[0:1], destinationList...)
				}
				// whereTo == "right"
				return append(destinationList, sourceList[0])
			}(),
		})
	case "right":
		err = params.SetValues(params.Context, map[string]interface{}{
			source: append([]interface{}{}, sourceList[:len(sourceList)-1]...),
			destination: func() []interface{} {
				if whereTo == "left" {
					return append(sourceList[len(sourceList)-1:], destinationList...)
				}
				// whereTo == "right"
				return append(destinationList, sourceList[len(sourceList)-1])
			}(),
		})
	}

	if err != nil {
		return nil, err
	}

	return []byte(constants.OkResponse), nil
}

func handleLPush(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := lpushKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	var newElems []interface{}

	for _, elem := range params.Command[2:] {
		newElems = append(newElems, internal.AdaptType(elem))
	}

	key := keys.WriteKeys[0]
	keyExists := params.KeysExist(params.Context, keys.WriteKeys)[key]

	if !keyExists {
		switch strings.ToLower(params.Command[0]) {
		case "lpushx":
			return nil, errors.New("LPUSHX command on non-existent key")
		default:
			if err = params.SetValues(params.Context, map[string]interface{}{key: []interface{}{}}); err != nil {
				return nil, err
			}
		}
	}

	currentList := params.GetValues(params.Context, []string{key})[key]
	l, ok := currentList.([]interface{})
	if !ok {
		return nil, errors.New("LPUSH command on non-list item")
	}

	if err = params.SetValues(params.Context, map[string]interface{}{key: append(newElems, l...)}); err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf(":%d\r\n", len(l)+len(newElems))), nil
}

func handleRPush(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := rpushKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	keyExists := params.KeysExist(params.Context, keys.WriteKeys)[key]

	var newElems []interface{}

	for _, elem := range params.Command[2:] {
		newElems = append(newElems, internal.AdaptType(elem))
	}

	if !keyExists {
		switch strings.ToLower(params.Command[0]) {
		case "rpushx":
			return nil, errors.New("RPUSHX command on non-existent key")
		default:
			if err = params.SetValues(params.Context, map[string]interface{}{key: []interface{}{}}); err != nil {
				return nil, err
			}
		}
	}

	currentList := params.GetValues(params.Context, []string{key})[key]
	l, ok := currentList.([]interface{})
	if !ok {
		return nil, errors.New("RPUSH command on non-list item")
	}

	if err = params.SetValues(params.Context, map[string]interface{}{key: append(l, newElems...)}); err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf(":%d\r\n", len(l)+len(newElems))), nil
}

func handlePop(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := popKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	keyExists := params.KeysExist(params.Context, keys.WriteKeys)[key]

	if !keyExists {
		return nil, fmt.Errorf("%s command on non-list item", strings.ToUpper(params.Command[0]))
	}

	list, ok := params.GetValues(params.Context, []string{key})[key].([]interface{})
	if !ok {
		return nil, fmt.Errorf("%s command on non-list item", strings.ToUpper(params.Command[0]))
	}

	switch strings.ToLower(params.Command[0]) {
	default:
		if err = params.SetValues(params.Context, map[string]interface{}{key: list[1:]}); err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("+%v\r\n", list[0])), nil
	case "rpop":
		if err = params.SetValues(params.Context, map[string]interface{}{key: list[:len(list)-1]}); err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("+%v\r\n", list[len(list)-1])), nil
	}
}

func Commands() []internal.Command {
	return []internal.Command{
		{
			Command:    "lpush",
			Module:     constants.ListModule,
			Categories: []string{constants.ListCategory, constants.WriteCategory, constants.FastCategory},
			Description: `(LPUSH key element [element ...]) 
Prepends one or more values to the beginning of a list, creates the list if it does not exist.`,
			Sync:              true,
			KeyExtractionFunc: lpushKeyFunc,
			HandlerFunc:       handleLPush,
		},
		{
			Command:    "lpushx",
			Module:     constants.ListModule,
			Categories: []string{constants.ListCategory, constants.WriteCategory, constants.FastCategory},
			Description: `(LPUSHX key element [element ...]) 
Prepends a value to the beginning of a list only if the list exists.`,
			Sync:              true,
			KeyExtractionFunc: lpushKeyFunc,
			HandlerFunc:       handleLPush,
		},
		{
			Command:           "lpop",
			Module:            constants.ListModule,
			Categories:        []string{constants.ListCategory, constants.WriteCategory, constants.FastCategory},
			Description:       "(LPOP key) Removes and returns the first element of a list.",
			Sync:              true,
			KeyExtractionFunc: popKeyFunc,
			HandlerFunc:       handlePop,
		},
		{
			Command:           "llen",
			Module:            constants.ListModule,
			Categories:        []string{constants.ListCategory, constants.ReadCategory, constants.FastCategory},
			Description:       "(LLEN key) Return the length of a list.",
			Sync:              false,
			KeyExtractionFunc: llenKeyFunc,
			HandlerFunc:       handleLLen,
		},
		{
			Command:           "lrange",
			Module:            constants.ListModule,
			Categories:        []string{constants.ListCategory, constants.ReadCategory, constants.SlowCategory},
			Description:       "(LRANGE key start end) Return a range of elements between the given indices.",
			Sync:              false,
			KeyExtractionFunc: lrangeKeyFunc,
			HandlerFunc:       handleLRange,
		},
		{
			Command:           "lindex",
			Module:            constants.ListModule,
			Categories:        []string{constants.ListCategory, constants.ReadCategory, constants.FastCategory},
			Description:       "(LINDEX key index) Gets list element by index.",
			Sync:              false,
			KeyExtractionFunc: lindexKeyFunc,
			HandlerFunc:       handleLIndex,
		},
		{
			Command:           "lset",
			Module:            constants.ListModule,
			Categories:        []string{constants.ListCategory, constants.WriteCategory, constants.FastCategory},
			Description:       "(LSET key index element) Sets the value of an element in a list by its index.",
			Sync:              true,
			KeyExtractionFunc: lsetKeyFunc,
			HandlerFunc:       handleLSet,
		},
		{
			Command:           "ltrim",
			Module:            constants.ListModule,
			Categories:        []string{constants.ListCategory, constants.WriteCategory, constants.SlowCategory},
			Description:       "(LTRIM key start end) Trims a list using the specified range.",
			Sync:              true,
			KeyExtractionFunc: ltrimKeyFunc,
			HandlerFunc:       handleLTrim,
		},
		{
			Command:           "lrem",
			Module:            constants.ListModule,
			Categories:        []string{constants.ListCategory, constants.WriteCategory, constants.SlowCategory},
			Description:       "(LREM key count element) Remove <count> elements from list.",
			Sync:              true,
			KeyExtractionFunc: lremKeyFunc,
			HandlerFunc:       handleLRem,
		},
		{
			Command:    "lmove",
			Module:     constants.ListModule,
			Categories: []string{constants.ListCategory, constants.WriteCategory, constants.SlowCategory},
			Description: `(LMOVE source destination <LEFT | RIGHT> <LEFT | RIGHT>) 
Move element from one list to the other specifying left/right for both lists.`,
			Sync:              true,
			KeyExtractionFunc: lmoveKeyFunc,
			HandlerFunc:       handleLMove,
		},
		{
			Command:           "rpop",
			Module:            constants.ListModule,
			Categories:        []string{constants.ListCategory, constants.WriteCategory, constants.FastCategory},
			Description:       "(RPOP key) Removes and gets the last element in a list.",
			Sync:              true,
			KeyExtractionFunc: popKeyFunc,
			HandlerFunc:       handlePop,
		},
		{
			Command:           "rpush",
			Module:            constants.ListModule,
			Categories:        []string{constants.ListCategory, constants.WriteCategory, constants.FastCategory},
			Description:       "(RPUSH key element [element ...]) Appends one or multiple elements to the end of a list.",
			Sync:              true,
			KeyExtractionFunc: rpushKeyFunc,
			HandlerFunc:       handleRPush,
		},
		{
			Command:           "rpushx",
			Module:            constants.ListModule,
			Categories:        []string{constants.ListCategory, constants.WriteCategory, constants.FastCategory},
			Description:       "(RPUSHX key element [element ...]) Appends an element to the end of a list, only if the list exists.",
			Sync:              true,
			KeyExtractionFunc: rpushKeyFunc,
			HandlerFunc:       handleRPush,
		},
	}
}
