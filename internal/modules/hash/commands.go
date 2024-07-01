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

package hash

import (
	"errors"
	"fmt"
	"github.com/echovault/echovault/internal"
	"github.com/echovault/echovault/internal/constants"
	"math/rand"
	"slices"
	"strconv"
	"strings"
)

func handleHSET(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := hsetKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	keyExists := params.KeysExist(params.Context, keys.WriteKeys)[key]
	entries := make(map[string]interface{})

	if len(params.Command[2:])%2 != 0 {
		return nil, errors.New("each field must have a corresponding value")
	}

	for i := 2; i <= len(params.Command)-2; i += 2 {
		entries[params.Command[i]] = internal.AdaptType(params.Command[i+1])
	}

	if !keyExists {
		if err != nil {
			return nil, err
		}
		if err = params.SetValues(params.Context, map[string]interface{}{key: entries}); err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf(":%d\r\n", len(entries))), nil
	}

	hash, ok := params.GetValues(params.Context, []string{key})[key].(map[string]interface{})
	if !ok {
		hash = make(map[string]interface{})
	}

	count := 0
	for field, value := range entries {
		if strings.EqualFold(params.Command[0], "hsetnx") {
			if hash[field] == nil {
				hash[field] = value
				count += 1
			}
			continue
		}
		hash[field] = value
		count += 1
	}
	if err = params.SetValues(params.Context, map[string]interface{}{key: hash}); err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf(":%d\r\n", count)), nil
}

func handleHGET(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := hgetKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	keyExists := params.KeysExist(params.Context, keys.ReadKeys)[key]
	fields := params.Command[2:]

	if !keyExists {
		return []byte("$-1\r\n"), nil
	}

	hash, ok := params.GetValues(params.Context, []string{key})[key].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value at %s is not a hash", key)
	}

	var value interface{}

	res := fmt.Sprintf("*%d\r\n", len(fields))
	for _, field := range fields {
		value = hash[field]
		if value == nil {
			res += "$-1\r\n"
			continue
		}
		if s, ok := value.(string); ok {
			res += fmt.Sprintf("$%d\r\n%s\r\n", len(s), s)
			continue
		}
		if d, ok := value.(int); ok {
			res += fmt.Sprintf(":%d\r\n", d)
			continue
		}
		if f, ok := value.(float64); ok {
			fs := strconv.FormatFloat(f, 'f', -1, 64)
			res += fmt.Sprintf("$%d\r\n%s\r\n", len(fs), fs)
			continue
		}
		res += fmt.Sprintf("$-1\r\n")
	}

	return []byte(res), nil
}

func handleHSTRLEN(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := hstrlenKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	keyExists := params.KeysExist(params.Context, keys.ReadKeys)[key]
	fields := params.Command[2:]

	if !keyExists {
		return []byte("$-1\r\n"), nil
	}

	hash, ok := params.GetValues(params.Context, []string{key})[key].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value at %s is not a hash", key)
	}

	var value interface{}

	res := fmt.Sprintf("*%d\r\n", len(fields))
	for _, field := range fields {
		value = hash[field]
		if value == nil {
			res += ":0\r\n"
			continue
		}
		if s, ok := value.(string); ok {
			res += fmt.Sprintf(":%d\r\n", len(s))
			continue
		}
		if f, ok := value.(float64); ok {
			fs := strconv.FormatFloat(f, 'f', -1, 64)
			res += fmt.Sprintf(":%d\r\n", len(fs))
			continue
		}
		if d, ok := value.(int); ok {
			res += fmt.Sprintf(":%d\r\n", len(strconv.Itoa(d)))
			continue
		}
		res += ":0\r\n"
	}

	return []byte(res), nil
}

func handleHVALS(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := hvalsKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	keyExists := params.KeysExist(params.Context, keys.ReadKeys)[key]

	if !keyExists {
		return []byte("*0\r\n"), nil
	}

	hash, ok := params.GetValues(params.Context, []string{key})[key].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value at %s is not a hash", key)
	}

	res := fmt.Sprintf("*%d\r\n", len(hash))
	for _, val := range hash {
		if s, ok := val.(string); ok {
			res += fmt.Sprintf("$%d\r\n%s\r\n", len(s), s)
			continue
		}
		if f, ok := val.(float64); ok {
			fs := strconv.FormatFloat(f, 'f', -1, 64)
			res += fmt.Sprintf("$%d\r\n%s\r\n", len(fs), fs)
			continue
		}
		if d, ok := val.(int); ok {
			res += fmt.Sprintf(":%d\r\n", d)
		}
	}

	return []byte(res), nil
}

func handleHRANDFIELD(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := hrandfieldKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	keyExists := params.KeysExist(params.Context, keys.ReadKeys)[key]

	count := 1
	if len(params.Command) >= 3 {
		c, err := strconv.Atoi(params.Command[2])
		if err != nil {
			return nil, errors.New("count must be an integer")
		}
		if c == 0 {
			return []byte("*0\r\n"), nil
		}
		count = c
	}

	withvalues := false
	if len(params.Command) == 4 {
		if strings.EqualFold(params.Command[3], "withvalues") {
			withvalues = true
		} else {
			return nil, errors.New("result modifier must be withvalues")
		}
	}

	if !keyExists {
		return []byte("*0\r\n"), nil
	}

	hash, ok := params.GetValues(params.Context, []string{key})[key].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value at %s is not a hash", key)
	}

	// If count is the >= hash length, then return the entire hash
	if count >= len(hash) {
		res := fmt.Sprintf("*%d\r\n", len(hash))
		if withvalues {
			res = fmt.Sprintf("*%d\r\n", len(hash)*2)
		}
		for field, value := range hash {
			res += fmt.Sprintf("$%d\r\n%s\r\n", len(field), field)
			if withvalues {
				if s, ok := value.(string); ok {
					res += fmt.Sprintf("$%d\r\n%s\r\n", len(s), s)
					continue
				}
				if f, ok := value.(float64); ok {
					fs := strconv.FormatFloat(f, 'f', -1, 64)
					res += fmt.Sprintf("$%d\r\n%s\r\n", len(fs), fs)
					continue
				}
				if d, ok := value.(int); ok {
					res += fmt.Sprintf(":%d\r\n", d)
					continue
				}
			}
		}
		return []byte(res), nil
	}

	// Get all the fields
	var fields []string
	for field, _ := range hash {
		fields = append(fields, field)
	}

	// Pluck fields and return them
	var pluckedFields []string
	var n int
	for i := 0; i < internal.AbsInt(count); i++ {
		n = rand.Intn(len(fields))
		pluckedFields = append(pluckedFields, fields[n])
		// If count is positive, remove the current field from list of fields
		if count > 0 {
			fields = slices.DeleteFunc(fields, func(s string) bool {
				return s == fields[n]
			})
		}
	}

	res := fmt.Sprintf("*%d\r\n", len(pluckedFields))
	if withvalues {
		res = fmt.Sprintf("*%d\r\n", len(pluckedFields)*2)
	}
	for _, field := range pluckedFields {
		res += fmt.Sprintf("$%d\r\n%s\r\n", len(field), field)
		if withvalues {
			if s, ok := hash[field].(string); ok {
				res += fmt.Sprintf("$%d\r\n%s\r\n", len(s), s)
				continue
			}
			if f, ok := hash[field].(float64); ok {
				fs := strconv.FormatFloat(f, 'f', -1, 64)
				res += fmt.Sprintf("$%d\r\n%s\r\n", len(fs), fs)
				continue
			}
			if d, ok := hash[field].(int); ok {
				res += fmt.Sprintf(":%d\r\n", d)
				continue
			}
		}
	}

	return []byte(res), nil
}

func handleHLEN(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := hlenKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	keyExists := params.KeysExist(params.Context, keys.ReadKeys)[key]

	if !keyExists {
		return []byte(":0\r\n"), nil
	}

	hash, ok := params.GetValues(params.Context, []string{key})[key].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value at %s is not a hash", key)
	}

	return []byte(fmt.Sprintf(":%d\r\n", len(hash))), nil
}

func handleHKEYS(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := hkeysKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	keyExists := params.KeysExist(params.Context, keys.ReadKeys)[key]

	if !keyExists {
		return []byte("*0\r\n"), nil
	}

	hash, ok := params.GetValues(params.Context, []string{key})[key].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value at %s is not a hash", key)
	}

	res := fmt.Sprintf("*%d\r\n", len(hash))
	for field, _ := range hash {
		res += fmt.Sprintf("$%d\r\n%s\r\n", len(field), field)
	}

	return []byte(res), nil
}

func handleHINCRBY(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := hincrbyKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	keyExists := params.KeysExist(params.Context, keys.WriteKeys)[key]
	field := params.Command[2]

	var intIncrement int
	var floatIncrement float64

	if strings.EqualFold(params.Command[0], "hincrbyfloat") {
		f, err := strconv.ParseFloat(params.Command[3], 64)
		if err != nil {
			return nil, errors.New("increment must be a float")
		}
		floatIncrement = f
	} else {
		i, err := strconv.Atoi(params.Command[3])
		if err != nil {
			return nil, errors.New("increment must be an integer")
		}
		intIncrement = i
	}

	if !keyExists {
		hash := make(map[string]interface{})
		if strings.EqualFold(params.Command[0], "hincrbyfloat") {
			hash[field] = floatIncrement
			if err = params.SetValues(params.Context, map[string]interface{}{key: hash}); err != nil {
				return nil, err
			}
			return []byte(fmt.Sprintf("+%s\r\n", strconv.FormatFloat(floatIncrement, 'f', -1, 64))), nil
		} else {
			hash[field] = intIncrement
			if err = params.SetValues(params.Context, map[string]interface{}{key: hash}); err != nil {
				return nil, err
			}
			return []byte(fmt.Sprintf(":%d\r\n", intIncrement)), nil
		}
	}

	hash, ok := params.GetValues(params.Context, []string{key})[key].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value at %s is not a hash", key)
	}

	if hash[field] == nil {
		hash[field] = 0
	}

	switch hash[field].(type) {
	default:
		return nil, fmt.Errorf("value at field %s is not a number", field)
	case int:
		i, _ := hash[field].(int)
		if strings.EqualFold(params.Command[0], "hincrbyfloat") {
			hash[field] = float64(i) + floatIncrement
		} else {
			hash[field] = i + intIncrement
		}
	case float64:
		f, _ := hash[field].(float64)
		if strings.EqualFold(params.Command[0], "hincrbyfloat") {
			hash[field] = f + floatIncrement
		} else {
			hash[field] = f + float64(intIncrement)
		}
	}

	if err = params.SetValues(params.Context, map[string]interface{}{key: hash}); err != nil {
		return nil, err
	}

	if f, ok := hash[field].(float64); ok {
		return []byte(fmt.Sprintf("+%s\r\n", strconv.FormatFloat(f, 'f', -1, 64))), nil
	}

	i, _ := hash[field].(int)
	return []byte(fmt.Sprintf(":%d\r\n", i)), nil
}

func handleHGETALL(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := hgetallKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	keyExists := params.KeysExist(params.Context, keys.ReadKeys)[key]

	if !keyExists {
		return []byte("*0\r\n"), nil
	}

	hash, ok := params.GetValues(params.Context, []string{key})[key].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value at %s is not a hash", key)
	}

	res := fmt.Sprintf("*%d\r\n", len(hash)*2)
	for field, value := range hash {
		res += fmt.Sprintf("$%d\r\n%s\r\n", len(field), field)
		if s, ok := value.(string); ok {
			res += fmt.Sprintf("$%d\r\n%s\r\n", len(s), s)
		}
		if f, ok := value.(float64); ok {
			fs := strconv.FormatFloat(f, 'f', -1, 64)
			res += fmt.Sprintf("$%d\r\n%s\r\n", len(fs), fs)
		}
		if d, ok := value.(int); ok {
			res += fmt.Sprintf(":%d\r\n", d)
		}
	}

	return []byte(res), nil
}

func handleHEXISTS(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := hexistsKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	keyExists := params.KeysExist(params.Context, keys.ReadKeys)[key]
	field := params.Command[2]

	if !keyExists {
		return []byte(":0\r\n"), nil
	}

	hash, ok := params.GetValues(params.Context, []string{key})[key].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value at %s is not a hash", key)
	}

	if hash[field] != nil {
		return []byte(":1\r\n"), nil
	}

	return []byte(":0\r\n"), nil
}

func handleHDEL(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := hdelKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	keyExists := params.KeysExist(params.Context, keys.WriteKeys)[key]
	fields := params.Command[2:]

	if !keyExists {
		return []byte(":0\r\n"), nil
	}

	hash, ok := params.GetValues(params.Context, []string{key})[key].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value at %s is not a hash", key)
	}

	count := 0

	for _, field := range fields {
		if hash[field] != nil {
			delete(hash, field)
			count += 1
		}
	}

	if err = params.SetValues(params.Context, map[string]interface{}{key: hash}); err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf(":%d\r\n", count)), nil
}

func Commands() []internal.Command {
	return []internal.Command{
		{
			Command:    "hset",
			Module:     constants.HashModule,
			Categories: []string{constants.HashCategory, constants.WriteCategory, constants.FastCategory},
			Description: `(HSET key field value [field value ...]) 
Set update each field of the hash with the corresponding value.`,
			Sync:              true,
			KeyExtractionFunc: hsetKeyFunc,
			HandlerFunc:       handleHSET,
		},
		{
			Command:    "hsetnx",
			Module:     constants.HashModule,
			Categories: []string{constants.HashCategory, constants.WriteCategory, constants.FastCategory},
			Description: `(HSETNX key field value [field value ...]) 
Set hash field value only if the field does not exist.`,
			Sync:              true,
			KeyExtractionFunc: hsetnxKeyFunc,
			HandlerFunc:       handleHSET,
		},
		{
			Command:    "hget",
			Module:     constants.HashModule,
			Categories: []string{constants.HashCategory, constants.ReadCategory, constants.FastCategory},
			Description: `(HGET key field [field ...]) 
Retrieve the value of each of the listed fields from the hash.`,
			Sync:              false,
			KeyExtractionFunc: hgetKeyFunc,
			HandlerFunc:       handleHGET,
		},
		{
			Command:    "hstrlen",
			Module:     constants.HashModule,
			Categories: []string{constants.HashCategory, constants.ReadCategory, constants.FastCategory},
			Description: `(HSTRLEN key field [field ...]) 
Return the string length of the values stored at the specified fields. 0 if the value does not exist.`,
			Sync:              false,
			KeyExtractionFunc: hstrlenKeyFunc,
			HandlerFunc:       handleHSTRLEN,
		},
		{
			Command:           "hvals",
			Module:            constants.HashModule,
			Categories:        []string{constants.HashCategory, constants.ReadCategory, constants.SlowCategory},
			Description:       `(HVALS key) Returns all the values of the hash at key.`,
			Sync:              false,
			KeyExtractionFunc: hvalsKeyFunc,
			HandlerFunc:       handleHVALS,
		},
		{
			Command:           "hrandfield",
			Module:            constants.HashModule,
			Categories:        []string{constants.HashCategory, constants.ReadCategory, constants.SlowCategory},
			Description:       `(HRANDFIELD key [count [WITHVALUES]]) Returns one or more random fields from the hash.`,
			Sync:              false,
			KeyExtractionFunc: hrandfieldKeyFunc,
			HandlerFunc:       handleHRANDFIELD,
		},
		{
			Command:           "hlen",
			Module:            constants.HashModule,
			Categories:        []string{constants.HashCategory, constants.ReadCategory, constants.FastCategory},
			Description:       `(HLEN key) Returns the number of fields in the hash.`,
			Sync:              false,
			KeyExtractionFunc: hlenKeyFunc,
			HandlerFunc:       handleHLEN,
		},
		{
			Command:           "hkeys",
			Module:            constants.HashModule,
			Categories:        []string{constants.HashCategory, constants.ReadCategory, constants.SlowCategory},
			Description:       `(HKEYS key) Returns all the fields in a hash.`,
			Sync:              false,
			KeyExtractionFunc: hkeysKeyFunc,
			HandlerFunc:       handleHKEYS,
		},
		{
			Command:           "hincrbyfloat",
			Module:            constants.HashModule,
			Categories:        []string{constants.HashCategory, constants.WriteCategory, constants.FastCategory},
			Description:       `(HINCRBYFLOAT key field increment) Increment the hash value by the float increment.`,
			Sync:              true,
			KeyExtractionFunc: hincrbyKeyFunc,
			HandlerFunc:       handleHINCRBY,
		},
		{
			Command:           "hincrby",
			Module:            constants.HashModule,
			Categories:        []string{constants.HashCategory, constants.WriteCategory, constants.FastCategory},
			Description:       `(HINCRBY key field increment) Increment the hash value by the integer increment`,
			Sync:              true,
			KeyExtractionFunc: hincrbyKeyFunc,
			HandlerFunc:       handleHINCRBY,
		},
		{
			Command:           "hgetall",
			Module:            constants.HashModule,
			Categories:        []string{constants.HashCategory, constants.ReadCategory, constants.SlowCategory},
			Description:       `(HGETALL key) Get all fields and values of a hash.`,
			Sync:              false,
			KeyExtractionFunc: hgetallKeyFunc,
			HandlerFunc:       handleHGETALL,
		},
		{
			Command:           "hexists",
			Module:            constants.HashModule,
			Categories:        []string{constants.HashCategory, constants.ReadCategory, constants.FastCategory},
			Description:       `(HEXISTS key field) Returns if field is an existing field in the hash.`,
			Sync:              false,
			KeyExtractionFunc: hexistsKeyFunc,
			HandlerFunc:       handleHEXISTS,
		},
		{
			Command:           "hdel",
			Module:            constants.HashModule,
			Categories:        []string{constants.HashCategory, constants.WriteCategory, constants.FastCategory},
			Description:       `(HDEL key field [field ...]) Deletes the specified fields from the hash.`,
			Sync:              true,
			KeyExtractionFunc: hdelKeyFunc,
			HandlerFunc:       handleHDEL,
		},
	}
}
