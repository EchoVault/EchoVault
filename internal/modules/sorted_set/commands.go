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

package sorted_set

import (
	"cmp"
	"errors"
	"fmt"
	"github.com/echovault/echovault/internal"
	"github.com/echovault/echovault/internal/constants"
	"math"
	"slices"
	"strconv"
	"strings"
)

func handleZADD(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := zaddKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	keyExists := params.KeysExist(params.Context, keys.WriteKeys)[key]

	var updatePolicy interface{} = nil
	var comparison interface{} = nil
	var changed interface{} = nil
	var incr interface{} = nil

	// Find the first valid score and this will be the start of the score/member pairs
	var membersStartIndex int
	for i := 0; i < len(params.Command); i++ {
		if membersStartIndex != 0 {
			break
		}
		switch internal.AdaptType(params.Command[i]).(type) {
		case string:
			if slices.Contains([]string{"-inf", "+inf"}, strings.ToLower(params.Command[i])) {
				membersStartIndex = i
			}
		case float64:
			membersStartIndex = i
		case int:
			membersStartIndex = i
		}
	}

	if membersStartIndex < 2 || len(params.Command[membersStartIndex:])%2 != 0 {
		return nil, errors.New("score/member pairs must be float/string")
	}

	var members []MemberParam

	for i := 0; i < len(params.Command[membersStartIndex:]); i++ {
		if i%2 != 0 {
			continue
		}
		score := internal.AdaptType(params.Command[membersStartIndex:][i])
		switch score.(type) {
		default:
			return nil, errors.New("invalid score in score/member list")
		case string:
			var s float64
			if strings.ToLower(score.(string)) == "-inf" {
				s = math.Inf(-1)
				members = append(members, MemberParam{
					Value: Value(params.Command[membersStartIndex:][i+1]),
					Score: Score(s),
				})
			}
			if strings.ToLower(score.(string)) == "+inf" {
				s = math.Inf(1)
				members = append(members, MemberParam{
					Value: Value(params.Command[membersStartIndex:][i+1]),
					Score: Score(s),
				})
			}
		case float64:
			s, _ := score.(float64)
			members = append(members, MemberParam{
				Value: Value(params.Command[membersStartIndex:][i+1]),
				Score: Score(s),
			})
		case int:
			s, _ := score.(int)
			members = append(members, MemberParam{
				Value: Value(params.Command[membersStartIndex:][i+1]),
				Score: Score(s),
			})
		}
	}

	// Parse options using membersStartIndex as the upper limit
	if membersStartIndex > 2 {
		options := params.Command[2:membersStartIndex]
		for _, option := range options {
			if slices.Contains([]string{"xx", "nx"}, strings.ToLower(option)) {
				updatePolicy = option
				// If option is "NX" and comparison is not nil, return an error
				if strings.EqualFold(option, "NX") && comparison != nil {
					return nil, errors.New("GT/LT flags not allowed if NX flag is provided")
				}
				continue
			}
			if slices.Contains([]string{"gt", "lt"}, strings.ToLower(option)) {
				comparison = option
				// If updatePolicy is "NX", return an error
				up, _ := updatePolicy.(string)
				if strings.EqualFold(up, "NX") {
					return nil, errors.New("GT/LT flags not allowed if NX flag is provided")
				}
				continue
			}
			if strings.EqualFold(option, "ch") {
				changed = option
				continue
			}
			if strings.EqualFold(option, "incr") {
				incr = option
				// If members length is more than 1, return an error
				if len(members) > 1 {
					return nil, errors.New("cannot pass more than one score/member pair when INCR flag is provided")
				}
				continue
			}
			return nil, fmt.Errorf("invalid option %s", option)
		}
	}

	if keyExists {
		// Key exists
		set, ok := params.GetValues(params.Context, []string{key})[key].(*SortedSet)
		if !ok {
			return nil, fmt.Errorf("value at %s is not a sorted set", key)
		}
		count, err := set.AddOrUpdate(members, updatePolicy, comparison, changed, incr)
		if err != nil {
			return nil, err
		}
		// If INCR option is provided, return the new score value
		if incr != nil {
			m := set.Get(members[0].Value)
			return []byte(fmt.Sprintf("+%f\r\n", m.Score)), nil
		}

		return []byte(fmt.Sprintf(":%d\r\n", count)), nil
	}

	// Key does not exist.
	set := NewSortedSet(members)
	if err = params.SetValues(params.Context, map[string]interface{}{key: set}); err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf(":%d\r\n", set.Cardinality())), nil
}

func handleZCARD(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := zcardKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	keyExists := params.KeysExist(params.Context, keys.ReadKeys)[key]

	if !keyExists {
		return []byte(":0\r\n"), nil
	}

	set, ok := params.GetValues(params.Context, []string{key})[key].(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	return []byte(fmt.Sprintf(":%d\r\n", set.Cardinality())), nil
}

func handleZCOUNT(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := zcountKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	keyExists := params.KeysExist(params.Context, keys.ReadKeys)[key]

	minimum := Score(math.Inf(-1))
	switch internal.AdaptType(params.Command[2]).(type) {
	default:
		return nil, errors.New("min constraint must be a double")
	case string:
		if strings.ToLower(params.Command[2]) == "+inf" {
			minimum = Score(math.Inf(1))
		} else {
			return nil, errors.New("min constraint must be a double")
		}
	case float64:
		s, _ := internal.AdaptType(params.Command[2]).(float64)
		minimum = Score(s)
	case int:
		s, _ := internal.AdaptType(params.Command[2]).(int)
		minimum = Score(s)
	}

	maximum := Score(math.Inf(1))
	switch internal.AdaptType(params.Command[3]).(type) {
	default:
		return nil, errors.New("max constraint must be a double")
	case string:
		if strings.ToLower(params.Command[3]) == "-inf" {
			maximum = Score(math.Inf(-1))
		} else {
			return nil, errors.New("max constraint must be a double")
		}
	case float64:
		s, _ := internal.AdaptType(params.Command[3]).(float64)
		maximum = Score(s)
	case int:
		s, _ := internal.AdaptType(params.Command[3]).(int)
		maximum = Score(s)
	}

	if !keyExists {
		return []byte(":0\r\n"), nil
	}

	set, ok := params.GetValues(params.Context, []string{key})[key].(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	var members []MemberParam
	for _, m := range set.GetAll() {
		if m.Score >= minimum && m.Score <= maximum {
			members = append(members, m)
		}
	}

	return []byte(fmt.Sprintf(":%d\r\n", len(members))), nil
}

func handleZLEXCOUNT(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := zlexcountKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	keyExists := params.KeysExist(params.Context, keys.ReadKeys)[key]
	minimum := params.Command[2]
	maximum := params.Command[3]

	if !keyExists {
		return []byte(":0\r\n"), nil
	}

	set, ok := params.GetValues(params.Context, []string{key})[key].(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	members := set.GetAll()

	// Check if all members has the same score
	for i := 0; i < len(members)-2; i++ {
		if members[i].Score != members[i+1].Score {
			return []byte(":0\r\n"), nil
		}
	}

	count := 0

	for _, m := range members {
		if slices.Contains([]int{1, 0}, internal.CompareLex(string(m.Value), minimum)) &&
			slices.Contains([]int{-1, 0}, internal.CompareLex(string(m.Value), maximum)) {
			count += 1
		}
	}

	return []byte(fmt.Sprintf(":%d\r\n", count)), nil
}

func handleZDIFF(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := zdiffKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	keyExists := params.KeysExist(params.Context, keys.ReadKeys)

	withscoresIndex := slices.IndexFunc(params.Command, func(s string) bool {
		return strings.EqualFold(s, "withscores")
	})
	if withscoresIndex > -1 && withscoresIndex < 2 {
		return nil, errors.New(constants.WrongArgsResponse)
	}

	// Extract base set
	if !keyExists[keys.ReadKeys[0]] {
		// If base set does not exist, return an empty array
		return []byte("*0\r\n"), nil
	}

	baseSortedSet, ok := params.GetValues(params.Context, []string{keys.ReadKeys[0]})[keys.ReadKeys[0]].(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", keys.ReadKeys[0])
	}

	// Extract the remaining sets
	var sets []*SortedSet

	for i := 1; i < len(keys.ReadKeys); i++ {
		if !keyExists[keys.ReadKeys[i]] {
			continue
		}
		set, ok := params.GetValues(params.Context, []string{keys.ReadKeys[i]})[keys.ReadKeys[i]].(*SortedSet)
		if !ok {
			return nil, fmt.Errorf("value at %s is not a sorted set", keys.ReadKeys[i])
		}
		sets = append(sets, set)
	}

	var diff = baseSortedSet.Subtract(sets)

	res := fmt.Sprintf("*%d", diff.Cardinality())
	includeScores := withscoresIndex != -1 && withscoresIndex >= 2

	for _, m := range diff.GetAll() {
		if includeScores {
			res += fmt.Sprintf("\r\n*2\r\n$%d\r\n%s\r\n+%s",
				len(m.Value), m.Value, strconv.FormatFloat(float64(m.Score), 'f', -1, 64))
		} else {
			res += fmt.Sprintf("\r\n*1\r\n$%d\r\n%s", len(m.Value), m.Value)
		}
	}

	res += "\r\n"

	return []byte(res), nil
}

func handleZDIFFSTORE(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := zdiffstoreKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	keyExists := params.KeysExist(params.Context, keys.ReadKeys)
	destination := keys.WriteKeys[0]

	// Extract base set
	if !keyExists[keys.ReadKeys[0]] {
		// If base set does not exist, return 0
		return []byte(":0\r\n"), nil
	}

	baseSortedSet, ok := params.GetValues(params.Context, []string{keys.ReadKeys[0]})[keys.ReadKeys[0]].(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", keys.ReadKeys[0])
	}

	var sets []*SortedSet

	for i := 1; i < len(keys.ReadKeys); i++ {
		if keyExists[keys.ReadKeys[i]] {
			set, ok := params.GetValues(params.Context, []string{keys.ReadKeys[i]})[keys.ReadKeys[i]].(*SortedSet)
			if !ok {
				return nil, fmt.Errorf("value at %s is not a sorted set", keys.ReadKeys[i])
			}
			sets = append(sets, set)
		}
	}

	diff := baseSortedSet.Subtract(sets)
	if err = params.SetValues(params.Context, map[string]interface{}{destination: diff}); err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf(":%d\r\n", diff.Cardinality())), nil
}

func handleZINCRBY(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := zincrbyKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	keyExists := params.KeysExist(params.Context, keys.WriteKeys)[key]

	member := Value(params.Command[3])
	var increment Score

	switch internal.AdaptType(params.Command[2]).(type) {
	default:
		return nil, errors.New("increment must be a double")
	case string:
		if strings.EqualFold("-inf", strings.ToLower(params.Command[2])) {
			increment = Score(math.Inf(-1))
		} else if strings.EqualFold("+inf", strings.ToLower(params.Command[2])) {
			increment = Score(math.Inf(1))
		} else {
			return nil, errors.New("increment must be a double")
		}
	case float64:
		s, _ := internal.AdaptType(params.Command[2]).(float64)
		increment = Score(s)
	case int:
		s, _ := internal.AdaptType(params.Command[2]).(int)
		increment = Score(s)
	}

	if !keyExists {
		// If the key does not exist, create a new sorted set at the key with
		// the member and increment as the first value
		if err = params.SetValues(
			params.Context,
			map[string]interface{}{
				key: NewSortedSet([]MemberParam{{Value: member, Score: increment}}),
			},
		); err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("+%s\r\n", strconv.FormatFloat(float64(increment), 'f', -1, 64))), nil
	}

	set, ok := params.GetValues(params.Context, []string{key})[key].(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}
	if _, err = set.AddOrUpdate(
		[]MemberParam{
			{Value: member, Score: increment}},
		"xx",
		nil,
		nil,
		"incr"); err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf("+%s\r\n",
		strconv.FormatFloat(float64(set.Get(member).Score), 'f', -1, 64))), nil
}

func handleZINTER(params internal.HandlerFuncParams) ([]byte, error) {
	_, err := zinterKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	keys, weights, aggregate, withscores, err := extractKeysWeightsAggregateWithScores(params.Command)
	if err != nil {
		return nil, err
	}
	keyExists := params.KeysExist(params.Context, keys)

	var setParams []SortedSetParam

	values := params.GetValues(params.Context, keys)
	for i := 0; i < len(keys); i++ {
		if !keyExists[keys[i]] {
			// If any of the keys is non-existent, return an empty array as there's no intersect
			return []byte("*0\r\n"), nil
		}
		set, ok := values[keys[i]].(*SortedSet)
		if !ok {
			return nil, fmt.Errorf("value at %s is not a sorted set", keys[i])
		}
		setParams = append(setParams, SortedSetParam{
			Set:    set,
			Weight: weights[i],
		})
	}

	intersect := Intersect(aggregate, setParams...)

	res := fmt.Sprintf("*%d", intersect.Cardinality())

	if intersect.Cardinality() > 0 {
		for _, m := range intersect.GetAll() {
			if withscores {
				res += fmt.Sprintf("\r\n*2\r\n$%d\r\n%s\r\n+%s", len(m.Value), m.Value, strconv.FormatFloat(float64(m.Score), 'f', -1, 64))
			} else {
				res += fmt.Sprintf("\r\n*1\r\n$%d\r\n%s", len(m.Value), m.Value)
			}
		}
	}

	res += "\r\n"

	return []byte(res), nil
}

func handleZINTERSTORE(params internal.HandlerFuncParams) ([]byte, error) {
	k, err := zinterstoreKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	keyExists := params.KeysExist(params.Context, k.ReadKeys)
	destination := k.WriteKeys[0]

	// Remove the destination keys from the command before parsing it
	cmd := slices.DeleteFunc(params.Command, func(s string) bool {
		return s == destination
	})

	keys, weights, aggregate, _, err := extractKeysWeightsAggregateWithScores(cmd)
	if err != nil {
		return nil, err
	}

	var setParams []SortedSetParam

	values := params.GetValues(params.Context, keys)
	for i := 0; i < len(keys); i++ {
		if !keyExists[keys[i]] {
			return []byte(":0\r\n"), nil
		}
		set, ok := values[keys[i]].(*SortedSet)
		if !ok {
			return nil, fmt.Errorf("value at %s is not a sorted set", keys[i])
		}
		setParams = append(setParams, SortedSetParam{
			Set:    set,
			Weight: weights[i],
		})
	}

	intersect := Intersect(aggregate, setParams...)
	if err = params.SetValues(params.Context, map[string]interface{}{
		destination: intersect,
	}); err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf(":%d\r\n", intersect.Cardinality())), nil
}

func handleZMPOP(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := zmpopKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	keyExists := params.KeysExist(params.Context, keys.WriteKeys)

	count := 1
	policy := "min"
	modifierIdx := -1

	// Parse COUNT from command
	countIdx := slices.IndexFunc(params.Command, func(s string) bool {
		return strings.ToLower(s) == "count"
	})
	if countIdx != -1 {
		if countIdx < 2 {
			return nil, errors.New(constants.WrongArgsResponse)
		}
		if countIdx == len(params.Command)-1 {
			return nil, errors.New("count must be a positive integer")
		}
		c, err := strconv.Atoi(params.Command[countIdx+1])
		if err != nil {
			return nil, err
		}
		if c <= 0 {
			return nil, errors.New("count must be a positive integer")
		}
		count = c
		modifierIdx = countIdx
	}

	// Parse MIN/MAX from the command
	policyIdx := slices.IndexFunc(params.Command, func(s string) bool {
		return slices.Contains([]string{"min", "max"}, strings.ToLower(s))
	})
	if policyIdx != -1 {
		if policyIdx < 2 {
			return nil, errors.New(constants.WrongArgsResponse)
		}
		policy = strings.ToLower(params.Command[policyIdx])
		if modifierIdx == -1 || (policyIdx < modifierIdx) {
			modifierIdx = policyIdx
		}
	}

	for i := 0; i < len(keys.WriteKeys); i++ {
		if keyExists[keys.WriteKeys[i]] {
			v, ok := params.GetValues(params.Context, []string{keys.WriteKeys[i]})[keys.WriteKeys[i]].(*SortedSet)
			if !ok || v.Cardinality() == 0 {
				continue
			}
			popped, err := v.Pop(count, policy)
			if err != nil {
				return nil, err
			}

			res := fmt.Sprintf("*%d", popped.Cardinality())

			for _, m := range popped.GetAll() {
				res += fmt.Sprintf("\r\n*2\r\n$%d\r\n%s\r\n+%s", len(m.Value), m.Value, strconv.FormatFloat(float64(m.Score), 'f', -1, 64))
			}

			res += "\r\n"

			return []byte(res), nil
		}
	}

	return []byte("*0\r\n"), nil
}

func handleZPOP(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := zpopKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	keyExists := params.KeysExist(params.Context, keys.WriteKeys)[key]
	count := 1
	policy := "min"

	if strings.EqualFold(params.Command[0], "zpopmax") {
		policy = "max"
	}

	if len(params.Command) == 3 {
		c, err := strconv.Atoi(params.Command[2])
		if err != nil {
			return nil, err
		}
		if c > 0 {
			count = c
		}
	}

	if !keyExists {
		return []byte("*0\r\n"), nil
	}

	set, ok := params.GetValues(params.Context, []string{key})[key].(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at key %s is not a sorted set", key)
	}

	popped, err := set.Pop(count, policy)
	if err != nil {
		return nil, err
	}

	res := fmt.Sprintf("*%d", popped.Cardinality())
	for _, m := range popped.GetAll() {
		res += fmt.Sprintf("\r\n*2\r\n$%d\r\n%s\r\n+%s",
			len(m.Value), m.Value, strconv.FormatFloat(float64(m.Score), 'f', -1, 64))
	}

	res += "\r\n"

	return []byte(res), nil
}

func handleZMSCORE(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := zmscoreKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	keyExists := params.KeysExist(params.Context, keys.ReadKeys)[key]

	if !keyExists {
		return []byte("*0\r\n"), nil
	}

	set, ok := params.GetValues(params.Context, []string{key})[key].(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	members := params.Command[2:]

	res := fmt.Sprintf("*%d", len(members))

	var member MemberObject

	for i := 0; i < len(members); i++ {
		member = set.Get(Value(members[i]))
		if !member.Exists {
			res = fmt.Sprintf("%s\r\n$-1", res)
		} else {
			res = fmt.Sprintf("%s\r\n+%s", res, strconv.FormatFloat(float64(member.Score), 'f', -1, 64))
		}
	}

	res += "\r\n"

	return []byte(res), nil
}

func handleZRANDMEMBER(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := zrandmemberKeyFunc(params.Command)
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
		if c != 0 {
			count = c
		}
	}

	withscores := false
	if len(params.Command) == 4 {
		if strings.EqualFold(params.Command[3], "withscores") {
			withscores = true
		} else {
			return nil, errors.New("last option must be WITHSCORES")
		}
	}

	if !keyExists {
		return []byte("$-1\r\n"), nil
	}

	set, ok := params.GetValues(params.Context, []string{key})[key].(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	members := set.GetRandom(count)

	res := fmt.Sprintf("*%d", len(members))
	for _, m := range members {
		if withscores {
			res += fmt.Sprintf("\r\n*2\r\n$%d\r\n%s\r\n+%s", len(m.Value), m.Value, strconv.FormatFloat(float64(m.Score), 'f', -1, 64))
		} else {
			res += fmt.Sprintf("\r\n*1\r\n$%d\r\n%s", len(m.Value), m.Value)
		}
	}

	res += "\r\n"

	return []byte(res), nil
}

func handleZRANK(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := zrankKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	keyExists := params.KeysExist(params.Context, keys.ReadKeys)[key]
	member := params.Command[2]
	withscores := false

	if len(params.Command) == 4 && strings.EqualFold(params.Command[3], "withscores") {
		withscores = true
	}

	if !keyExists {
		return []byte("$-1\r\n"), nil
	}

	set, ok := params.GetValues(params.Context, []string{key})[key].(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	members := set.GetAll()
	slices.SortFunc(members, func(a, b MemberParam) int {
		if strings.EqualFold(params.Command[0], "zrevrank") {
			return cmp.Compare(b.Score, a.Score)
		}
		return cmp.Compare(a.Score, b.Score)
	})

	for i := 0; i < len(members); i++ {
		if members[i].Value == Value(member) {
			if withscores {
				score := strconv.FormatFloat(float64(members[i].Score), 'f', -1, 64)
				return []byte(fmt.Sprintf("*2\r\n:%d\r\n$%d\r\n%s\r\n", i, len(score), score)), nil
			} else {
				return []byte(fmt.Sprintf("*1\r\n:%d\r\n", i)), nil
			}
		}
	}

	return []byte("$-1\r\n"), nil
}

func handleZREM(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := zremKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	keyExists := params.KeysExist(params.Context, keys.WriteKeys)[key]

	if !keyExists {
		return []byte(":0\r\n"), nil
	}

	set, ok := params.GetValues(params.Context, []string{key})[key].(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	deletedCount := 0
	for _, m := range params.Command[2:] {
		if set.Remove(Value(m)) {
			deletedCount += 1
		}
	}

	return []byte(fmt.Sprintf(":%d\r\n", deletedCount)), nil
}

func handleZSCORE(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := zscoreKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	keyExists := params.KeysExist(params.Context, keys.ReadKeys)[key]

	if !keyExists {
		return []byte("$-1\r\n"), nil
	}

	set, ok := params.GetValues(params.Context, []string{key})[key].(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}
	member := set.Get(Value(params.Command[2]))
	if !member.Exists {
		return []byte("$-1\r\n"), nil
	}

	score := strconv.FormatFloat(float64(member.Score), 'f', -1, 64)

	return []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(score), score)), nil
}

func handleZREMRANGEBYSCORE(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := zremrangebyscoreKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	keyExists := params.KeysExist(params.Context, keys.WriteKeys)[key]

	deletedCount := 0

	minimum, err := strconv.ParseFloat(params.Command[2], 64)
	if err != nil {
		return nil, err
	}

	maximum, err := strconv.ParseFloat(params.Command[3], 64)
	if err != nil {
		return nil, err
	}

	if !keyExists {
		return []byte(":0\r\n"), nil
	}

	set, ok := params.GetValues(params.Context, []string{key})[key].(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	for _, m := range set.GetAll() {
		if m.Score >= Score(minimum) && m.Score <= Score(maximum) {
			set.Remove(m.Value)
			deletedCount += 1
		}
	}

	return []byte(fmt.Sprintf(":%d\r\n", deletedCount)), nil
}

func handleZREMRANGEBYRANK(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := zremrangebyrankKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	keyExists := params.KeysExist(params.Context, keys.WriteKeys)[key]

	start, err := strconv.Atoi(params.Command[2])
	if err != nil {
		return nil, err
	}

	stop, err := strconv.Atoi(params.Command[3])
	if err != nil {
		return nil, err
	}

	if !keyExists {
		return []byte(":0\r\n"), nil
	}

	set, ok := params.GetValues(params.Context, []string{key})[key].(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	if start < 0 {
		start = start + set.Cardinality()
	}
	if stop < 0 {
		stop = stop + set.Cardinality()
	}

	if start < 0 || start > set.Cardinality()-1 || stop < 0 || stop > set.Cardinality()-1 {
		return nil, errors.New("indices out of bounds")
	}

	members := set.GetAll()
	slices.SortFunc(members, func(a, b MemberParam) int {
		return cmp.Compare(a.Score, b.Score)
	})

	deletedCount := 0

	if start < stop {
		for i := start; i <= stop; i++ {
			set.Remove(members[i].Value)
			deletedCount += 1
		}
	} else {
		for i := stop; i <= start; i++ {
			set.Remove(members[i].Value)
			deletedCount += 1
		}
	}

	return []byte(fmt.Sprintf(":%d\r\n", deletedCount)), nil
}

func handleZREMRANGEBYLEX(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := zremrangebylexKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	keyExists := params.KeysExist(params.Context, keys.WriteKeys)[key]
	minimum := params.Command[2]
	maximum := params.Command[3]

	if !keyExists {
		return []byte(":0\r\n"), nil
	}

	set, ok := params.GetValues(params.Context, []string{key})[key].(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	members := set.GetAll()

	// Check if all the members have the same score. If not, return 0
	for i := 0; i < len(members)-1; i++ {
		if members[i].Score != members[i+1].Score {
			return []byte(":0\r\n"), nil
		}
	}

	deletedCount := 0

	// All the members have the same score
	for _, m := range members {
		if slices.Contains([]int{1, 0}, internal.CompareLex(string(m.Value), minimum)) &&
			slices.Contains([]int{-1, 0}, internal.CompareLex(string(m.Value), maximum)) {
			set.Remove(m.Value)
			deletedCount += 1
		}
	}

	return []byte(fmt.Sprintf(":%d\r\n", deletedCount)), nil
}

func handleZRANGE(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := zrangeKeyCount(params.Command)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	keyExists := params.KeysExist(params.Context, keys.ReadKeys)[key]

	policy := "byscore"
	scoreStart := math.Inf(-1)    // Lower bound if policy is "byscore"
	scoreStop := math.Inf(1)      // Upper bound if policy is "byscore"
	lexStart := params.Command[2] // Lower bound if policy is "bylex"
	lexStop := params.Command[3]  // Upper bound if policy is "bylex"
	offset := 0
	count := -1

	withscores := slices.ContainsFunc(params.Command[4:], func(s string) bool {
		return strings.EqualFold(s, "withscores")
	})

	reverse := slices.ContainsFunc(params.Command[4:], func(s string) bool {
		return strings.EqualFold(s, "rev")
	})

	if slices.ContainsFunc(params.Command[4:], func(s string) bool {
		return strings.EqualFold(s, "bylex")
	}) {
		policy = "bylex"
	} else {
		// policy is "byscore" make sure start and stop are valid float values
		scoreStart, err = strconv.ParseFloat(params.Command[2], 64)
		if err != nil {
			return nil, err
		}
		scoreStop, err = strconv.ParseFloat(params.Command[3], 64)
		if err != nil {
			return nil, err
		}
	}

	if slices.ContainsFunc(params.Command[4:], func(s string) bool {
		return strings.EqualFold(s, "limit")
	}) {
		limitIdx := slices.IndexFunc(params.Command[4:], func(s string) bool {
			return strings.EqualFold(s, "limit")
		})
		if limitIdx != -1 && limitIdx > len(params.Command[4:])-3 {
			return nil, errors.New("limit should contain offset and count as integers")
		}
		offset, err = strconv.Atoi(params.Command[4:][limitIdx+1])
		if err != nil {
			return nil, errors.New("limit offset must be integer")
		}
		if offset < 0 {
			return nil, errors.New("limit offset must be >= 0")
		}
		count, err = strconv.Atoi(params.Command[4:][limitIdx+2])
		if err != nil {
			return nil, errors.New("limit count must be integer")
		}
	}

	if !keyExists {
		return []byte("*0\r\n"), nil
	}

	set, ok := params.GetValues(params.Context, []string{key})[key].(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	if offset > set.Cardinality() {
		return []byte("*0\r\n"), nil
	}
	if count < 0 {
		count = set.Cardinality() - offset
	}

	members := set.GetAll()
	if strings.EqualFold(policy, "byscore") {
		slices.SortFunc(members, func(a, b MemberParam) int {
			// Do a score sort
			if reverse {
				return cmp.Compare(b.Score, a.Score)
			}
			return cmp.Compare(a.Score, b.Score)
		})
	}
	if strings.EqualFold(policy, "bylex") {
		// If policy is BYLEX, all the elements must have the same score
		for i := 0; i < len(members)-1; i++ {
			if members[i].Score != members[i+1].Score {
				return []byte("*0\r\n"), nil
			}
		}
		slices.SortFunc(members, func(a, b MemberParam) int {
			if reverse {
				return internal.CompareLex(string(b.Value), string(a.Value))
			}
			return internal.CompareLex(string(a.Value), string(b.Value))
		})
	}

	var resultMembers []MemberParam

	for i := offset; i <= count; i++ {
		if i >= len(members) {
			break
		}
		if strings.EqualFold(policy, "byscore") {
			if members[i].Score >= Score(scoreStart) && members[i].Score <= Score(scoreStop) {
				resultMembers = append(resultMembers, members[i])
			}
			continue
		}
		if slices.Contains([]int{1, 0}, internal.CompareLex(string(members[i].Value), lexStart)) &&
			slices.Contains([]int{-1, 0}, internal.CompareLex(string(members[i].Value), lexStop)) {
			resultMembers = append(resultMembers, members[i])
		}
	}

	res := fmt.Sprintf("*%d", len(resultMembers))

	for _, m := range resultMembers {
		if withscores {
			res += fmt.Sprintf("\r\n*2\r\n$%d\r\n%s\r\n+%s", len(m.Value), m.Value, strconv.FormatFloat(float64(m.Score), 'f', -1, 64))
		} else {
			res += fmt.Sprintf("\r\n*1\r\n$%d\r\n%s", len(m.Value), m.Value)
		}
	}

	res += "\r\n"

	return []byte(res), nil
}

func handleZRANGESTORE(params internal.HandlerFuncParams) ([]byte, error) {
	keys, err := zrangeStoreKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	destination := keys.WriteKeys[0]
	source := keys.ReadKeys[0]
	sourceExists := params.KeysExist(params.Context, keys.ReadKeys)[source]
	policy := "byscore"
	scoreStart := math.Inf(-1)    // Lower bound if policy is "byscore"
	scoreStop := math.Inf(1)      // Upper bound if policy is "byfloat"
	lexStart := params.Command[3] // Lower bound if policy is "bylex"
	lexStop := params.Command[4]  // Upper bound if policy is "bylex"
	offset := 0
	count := -1

	reverse := slices.ContainsFunc(params.Command[5:], func(s string) bool {
		return strings.EqualFold(s, "rev")
	})

	if slices.ContainsFunc(params.Command[5:], func(s string) bool {
		return strings.EqualFold(s, "bylex")
	}) {
		policy = "bylex"
	} else {
		// policy is "byscore" make sure start and stop are valid float values
		scoreStart, err = strconv.ParseFloat(params.Command[3], 64)
		if err != nil {
			return nil, err
		}
		scoreStop, err = strconv.ParseFloat(params.Command[4], 64)
		if err != nil {
			return nil, err
		}
	}

	if slices.ContainsFunc(params.Command[5:], func(s string) bool {
		return strings.EqualFold(s, "limit")
	}) {
		limitIdx := slices.IndexFunc(params.Command[5:], func(s string) bool {
			return strings.EqualFold(s, "limit")
		})
		if limitIdx != -1 && limitIdx > len(params.Command[5:])-3 {
			return nil, errors.New("limit should contain offset and count as integers")
		}
		offset, err = strconv.Atoi(params.Command[5:][limitIdx+1])
		if err != nil {
			return nil, errors.New("limit offset must be integer")
		}
		if offset < 0 {
			return nil, errors.New("limit offset must be >= 0")
		}
		count, err = strconv.Atoi(params.Command[5:][limitIdx+2])
		if err != nil {
			return nil, errors.New("limit count must be integer")
		}
	}

	if !sourceExists {
		return []byte("*0\r\n"), nil
	}

	set, ok := params.GetValues(params.Context, []string{source})[source].(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", source)
	}

	if offset > set.Cardinality() {
		return []byte(":0\r\n"), nil
	}
	if count < 0 {
		count = set.Cardinality() - offset
	}

	members := set.GetAll()
	if strings.EqualFold(policy, "byscore") {
		slices.SortFunc(members, func(a, b MemberParam) int {
			// Do a score sort
			if reverse {
				return cmp.Compare(b.Score, a.Score)
			}
			return cmp.Compare(a.Score, b.Score)
		})
	}
	if strings.EqualFold(policy, "bylex") {
		// If policy is BYLEX, all the elements must have the same score
		for i := 0; i < len(members)-1; i++ {
			if members[i].Score != members[i+1].Score {
				return []byte(":0\r\n"), nil
			}
		}
		slices.SortFunc(members, func(a, b MemberParam) int {
			if reverse {
				return internal.CompareLex(string(b.Value), string(a.Value))
			}
			return internal.CompareLex(string(a.Value), string(b.Value))
		})
	}

	var resultMembers []MemberParam

	for i := offset; i <= count; i++ {
		if i >= len(members) {
			break
		}
		if strings.EqualFold(policy, "byscore") {
			if members[i].Score >= Score(scoreStart) && members[i].Score <= Score(scoreStop) {
				resultMembers = append(resultMembers, members[i])
			}
			continue
		}
		if slices.Contains([]int{1, 0}, internal.CompareLex(string(members[i].Value), lexStart)) &&
			slices.Contains([]int{-1, 0}, internal.CompareLex(string(members[i].Value), lexStop)) {
			resultMembers = append(resultMembers, members[i])
		}
	}

	newSortedSet := NewSortedSet(resultMembers)
	if err = params.SetValues(params.Context, map[string]interface{}{
		destination: newSortedSet,
	}); err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf(":%d\r\n", newSortedSet.Cardinality())), nil
}

func handleZUNION(params internal.HandlerFuncParams) ([]byte, error) {
	if _, err := zunionKeyFunc(params.Command); err != nil {
		return nil, err
	}

	keys, weights, aggregate, withscores, err := extractKeysWeightsAggregateWithScores(params.Command)
	if err != nil {
		return nil, err
	}

	keyExists := params.KeysExist(params.Context, keys)

	var setParams []SortedSetParam

	values := params.GetValues(params.Context, keys)
	for i := 0; i < len(keys); i++ {
		if keyExists[keys[i]] {
			set, ok := values[keys[i]].(*SortedSet)
			if !ok {
				return nil, fmt.Errorf("value at %s is not a sorted set", keys[i])
			}
			setParams = append(setParams, SortedSetParam{
				Set:    set,
				Weight: weights[i],
			})
		}
	}

	union := Union(aggregate, setParams...)

	res := fmt.Sprintf("*%d", union.Cardinality())
	for _, m := range union.GetAll() {
		if withscores {
			res += fmt.Sprintf("\r\n*2\r\n$%d\r\n%s\r\n+%s", len(m.Value), m.Value, strconv.FormatFloat(float64(m.Score), 'f', -1, 64))
		} else {
			res += fmt.Sprintf("\r\n*1\r\n$%d\r\n%s", len(m.Value), m.Value)
		}
	}

	res += "\r\n"

	return []byte(res), nil
}

func handleZUNIONSTORE(params internal.HandlerFuncParams) ([]byte, error) {
	k, err := zunionstoreKeyFunc(params.Command)
	if err != nil {
		return nil, err
	}

	destination := k.WriteKeys[0]

	// Remove destination key from list of keys
	params.Command = slices.DeleteFunc(params.Command, func(s string) bool {
		return s == destination
	})

	keys, weights, aggregate, _, err := extractKeysWeightsAggregateWithScores(params.Command)
	if err != nil {
		return nil, err
	}

	keyExists := params.KeysExist(params.Context, keys)

	var setParams []SortedSetParam

	values := params.GetValues(params.Context, keys)
	for i := 0; i < len(keys); i++ {
		if keyExists[keys[i]] {
			set, ok := values[keys[i]].(*SortedSet)
			if !ok {
				return nil, fmt.Errorf("value at %s is not a sorted set", keys[i])
			}
			setParams = append(setParams, SortedSetParam{
				Set:    set,
				Weight: weights[i],
			})
		}
	}

	union := Union(aggregate, setParams...)
	if err = params.SetValues(params.Context, map[string]interface{}{
		destination: union,
	}); err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf(":%d\r\n", union.Cardinality())), nil
}

func Commands() []internal.Command {
	return []internal.Command{
		{
			Command:    "zadd",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.WriteCategory, constants.FastCategory},
			Description: `(ZADD key [NX | XX] [GT | LT] [CH] [INCR] score member [score member...])
Adds all the specified members with the specified scores to the sorted set at the key.
"NX" only adds the member if it currently does not exist in the sorted set.
"XX" only updates the scores of members that exist in the sorted set.
"GT"" only updates the score if the new score is greater than the current score.
"LT" only updates the score if the new score is less than the current score.
"CH" modifies the result to return total number of members changed + added, instead of only new members added.
"INCR" modifies the command to act like ZINCRBY, only one score/member pair can be specified in this mode.`,
			Sync:              true,
			KeyExtractionFunc: zaddKeyFunc,
			HandlerFunc:       handleZADD,
		},
		{
			Command:    "zcard",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.ReadCategory, constants.SlowCategory},
			Description: `(ZCARD key) Returns the set cardinality of the sorted set at key.
If the key does not exist, 0 is returned, otherwise the cardinality of the sorted set is returned.
If the key holds a value that is not a sorted set, this command will return an error.`,
			Sync:              false,
			KeyExtractionFunc: zcardKeyFunc,
			HandlerFunc:       handleZCARD,
		},
		{
			Command:    "zcount",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.ReadCategory, constants.SlowCategory},
			Description: `(ZCOUNT key min max) 
Returns the number of elements in the sorted set key with scores in the range of min and max.
If the key does not exist, a count of 0 is returned, otherwise return the count.
If the key holds a value that is not a sorted set, an error is returned.`,
			Sync:              false,
			KeyExtractionFunc: zcountKeyFunc,
			HandlerFunc:       handleZCOUNT,
		},
		{
			Command:    "zdiff",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.ReadCategory, constants.SlowCategory},
			Description: `(ZDIFF key [key...] [WITHSCORES]) 
Computes the difference between all the sorted sets specified in the list of keys and returns the result.`,
			Sync:              false,
			KeyExtractionFunc: zdiffKeyFunc,
			HandlerFunc:       handleZDIFF,
		},
		{
			Command:    "zdiffstore",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.WriteCategory, constants.SlowCategory},
			Description: `(ZDIFFSTORE destination key [key...]). 
Computes the difference between all the sorted sets specifies in the list of keys. Stores the result in destination.
If the base set (first key) does not exist, return 0, otherwise, return the cardinality of the diff.`,
			Sync:              true,
			KeyExtractionFunc: zdiffstoreKeyFunc,
			HandlerFunc:       handleZDIFFSTORE,
		},
		{
			Command:    "zincrby",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.WriteCategory, constants.FastCategory},
			Description: `(ZINCRBY key increment member). 
Increments the score of the specified sorted set's member by the increment. If the member does not exist, it is created.
If the key does not exist, it is created with new sorted set and the member added with the increment as its score.`,
			Sync:              true,
			KeyExtractionFunc: zincrbyKeyFunc,
			HandlerFunc:       handleZINCRBY,
		},
		{
			Command:    "zinter",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.ReadCategory, constants.SlowCategory},
			Description: `(ZINTER key [key ...] [WEIGHTS weight [weight ...]] [AGGREGATE <SUM | MIN | MAX>] [WITHSCORES]).
Computes the intersection of the sets in the keys, with weights, aggregate and scores`,
			Sync:              false,
			KeyExtractionFunc: zinterKeyFunc,
			HandlerFunc:       handleZINTER,
		},
		{
			Command:    "zinterstore",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.WriteCategory, constants.SlowCategory},
			Description: `
(ZINTERSTORE destination key [key ...] [WEIGHTS weight [weight ...]] [AGGREGATE <SUM | MIN | MAX>] [WITHSCORES]).
Computes the intersection of the sets in the keys, with weights, aggregate and scores. The result is stored in destination.`,
			Sync:              true,
			KeyExtractionFunc: zinterstoreKeyFunc,
			HandlerFunc:       handleZINTERSTORE,
		},
		{
			Command:    "zmpop",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.WriteCategory, constants.SlowCategory},
			Description: `(ZMPOP key [key ...] <MIN | MAX> [COUNT count])
Pop a 'count' elements from multiple sorted sets. MIN or MAX determines whether to pop elements with the lowest or highest scores
respectively.`,
			Sync:              true,
			KeyExtractionFunc: zmpopKeyFunc,
			HandlerFunc:       handleZMPOP,
		},
		{
			Command:    "zmscore",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.ReadCategory, constants.FastCategory},
			Description: `(ZMSCORE key member [member ...])
Returns the associated scores of the specified member in the sorted set. 
Returns nil for members that do not exist in the set`,
			Sync:              false,
			KeyExtractionFunc: zmscoreKeyFunc,
			HandlerFunc:       handleZMSCORE,
		},
		{
			Command:    "zpopmax",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.WriteCategory, constants.SlowCategory},
			Description: `(ZPOPMAX key [count])
Removes and returns 'count' number of members in the sorted set with the highest scores. Default count is 1.`,
			Sync:              true,
			KeyExtractionFunc: zpopKeyFunc,
			HandlerFunc:       handleZPOP,
		},
		{
			Command:    "zpopmin",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.WriteCategory, constants.SlowCategory},
			Description: `(ZPOPMIN key [count])
Removes and returns 'count' number of members in the sorted set with the lowest scores. Default count is 1.`,
			Sync:              true,
			KeyExtractionFunc: zpopKeyFunc,
			HandlerFunc:       handleZPOP,
		},
		{
			Command:    "zrandmember",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.ReadCategory, constants.SlowCategory},
			Description: `(ZRANDMEMBER key [count [WITHSCORES]])
Return a list of length equivalent to count containing random members of the sorted set.
If count is negative, repeated elements are allowed. If count is positive, the returned elements will be distinct.
WITHSCORES modifies the result to include scores in the result.`,
			Sync:              false,
			KeyExtractionFunc: zrandmemberKeyFunc,
			HandlerFunc:       handleZRANDMEMBER,
		},
		{
			Command:    "zrank",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.ReadCategory, constants.SlowCategory},
			Description: `(ZRANK key member [WITHSCORE])
Returns the rank of the specified member in the sorted set. WITHSCORE modifies the result to also return the score.`,
			Sync:              false,
			KeyExtractionFunc: zrankKeyFunc,
			HandlerFunc:       handleZRANK,
		},
		{
			Command:    "zrevrank",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.ReadCategory, constants.SlowCategory},
			Description: `(ZREVRANK key member [WITHSCORE])
Returns the rank of the member in the sorted set in reverse order. 
WITHSCORE modifies the result to include the score.`,
			Sync:              false,
			KeyExtractionFunc: zrevrankKeyFunc,
			HandlerFunc:       handleZRANK,
		},
		{
			Command:    "zrem",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.WriteCategory, constants.FastCategory},
			Description: `(ZREM key member [member ...]) Removes the listed members from the sorted set.
Returns the number of elements removed.`,
			Sync:              true,
			KeyExtractionFunc: zremKeyFunc,
			HandlerFunc:       handleZREM,
		},
		{
			Command:           "zscore",
			Module:            constants.SortedSetModule,
			Categories:        []string{constants.SortedSetCategory, constants.ReadCategory, constants.FastCategory},
			Description:       `(ZSCORE key member) Returns the score of the member in the sorted set.`,
			Sync:              false,
			KeyExtractionFunc: zscoreKeyFunc,
			HandlerFunc:       handleZSCORE,
		},
		{
			Command:           "zremrangebylex",
			Module:            constants.SortedSetModule,
			Categories:        []string{constants.SortedSetCategory, constants.WriteCategory, constants.SlowCategory},
			Description:       `(ZREMRANGEBYLEX key min max) Removes the elements in the lexicographical range between min and max`,
			Sync:              true,
			KeyExtractionFunc: zremrangebylexKeyFunc,
			HandlerFunc:       handleZREMRANGEBYLEX,
		},
		{
			Command:    "zremrangebyrank",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.WriteCategory, constants.SlowCategory},
			Description: `(ZREMRANGEBYRANK key start stop) Removes the elements in the rank range between start and stop.
The elements are ordered from lowest score to highest score`,
			Sync:              true,
			KeyExtractionFunc: zremrangebyrankKeyFunc,
			HandlerFunc:       handleZREMRANGEBYRANK,
		},
		{
			Command:           "zremrangebyscore",
			Module:            constants.SortedSetModule,
			Categories:        []string{constants.SortedSetCategory, constants.WriteCategory, constants.SlowCategory},
			Description:       `(ZREMRANGEBYSCORE key min max) Removes the elements whose scores are in the range between min and max`,
			Sync:              true,
			KeyExtractionFunc: zremrangebyscoreKeyFunc,
			HandlerFunc:       handleZREMRANGEBYSCORE,
		},
		{
			Command:    "zlexcount",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.ReadCategory, constants.SlowCategory},
			Description: `(ZLEXCOUNT key min max) Returns the number of elements in within the sorted set within the 
lexicographical range between min and max. Returns 0, if the keys does not exist or if all the members do not have
the same score. If the value held at key is not a sorted set, an error is returned.`,
			Sync:              false,
			KeyExtractionFunc: zlexcountKeyFunc,
			HandlerFunc:       handleZLEXCOUNT,
		},
		{
			Command:    "zrange",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.ReadCategory, constants.SlowCategory},
			Description: `(ZRANGE key start stop [BYSCORE | BYLEX] [REV] [LIMIT offset count]
  [WITHSCORES]) Returns the range of elements in the sorted set.`,
			Sync:              false,
			KeyExtractionFunc: zrangeKeyCount,
			HandlerFunc:       handleZRANGE,
		},
		{
			Command:    "zrangestore",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.WriteCategory, constants.SlowCategory},
			Description: `ZRANGESTORE destination source start stop [BYSCORE | BYLEX] [REV] [LIMIT offset count]
  [WITHSCORES] Retrieve the range of elements in the sorted set and store it in destination.`,
			Sync:              true,
			KeyExtractionFunc: zrangeStoreKeyFunc,
			HandlerFunc:       handleZRANGESTORE,
		},
		{
			Command:    "zunion",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.ReadCategory, constants.SlowCategory},
			Description: `(ZUNION key [key ...] [WEIGHTS weight [weight ...]]
[AGGREGATE <SUM | MIN | MAX>] [WITHSCORES]) Return the union of the sorted sets in keys. The scores of each member of 
a sorted set are multiplied by the corresponding weight in WEIGHTS. Aggregate determines how the scores are combined.
WITHSCORES option determines whether to return the result with scores included.`,
			Sync:              false,
			KeyExtractionFunc: zunionKeyFunc,
			HandlerFunc:       handleZUNION,
		},
		{
			Command:    "zunionstore",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.WriteCategory, constants.SlowCategory},
			Description: `(ZUNIONSTORE destination key [key ...] [WEIGHTS weight [weight ...]]
[AGGREGATE <SUM | MIN | MAX>] [WITHSCORES]) Return the union of the sorted sets in keys. The scores of each member of 
a sorted set are multiplied by the corresponding weight in WEIGHTS. Aggregate determines how the scores are combined.
The resulting union is stored at the destination key.`,
			Sync:              true,
			KeyExtractionFunc: zunionstoreKeyFunc,
			HandlerFunc:       handleZUNIONSTORE,
		},
	}
}
