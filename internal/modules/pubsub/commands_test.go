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

package pubsub_test

import (
	"net"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/echovault/sugardb/internal"
	"github.com/echovault/sugardb/internal/constants"
	"github.com/echovault/sugardb/pkg/config"
	"github.com/echovault/sugardb/sugardb"
	"github.com/tidwall/resp"
)

func setUpServer(port int) (*sugardb.SugarDB, error) {
	return sugardb.NewSugarDB(
		sugardb.WithConfig(config.Config{
			BindAddr:       "localhost",
			Port:           uint16(port),
			DataDir:        "",
			EvictionPolicy: constants.NoEviction,
		}),
	)
}

func Test_PubSub(t *testing.T) {
	port, err := internal.GetFreePort()
	if err != nil {
		t.Error(err)
		return
	}

	mockServer, err := setUpServer(port)
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

	t.Run("Test_HandleSubscribe", func(t *testing.T) {
		t.Parallel()

		// Establish connections.
		numOfConnections := 20
		rawConnections := make([]net.Conn, numOfConnections)
		connections := make([]*resp.Conn, numOfConnections)
		for i := 0; i < numOfConnections; i++ {
			conn, err := internal.GetConnection("localhost", port)
			if err != nil {
				t.Error(err)
				return
			}
			rawConnections[i] = conn
			connections[i] = resp.NewConn(conn)
		}
		defer func() {
			for _, conn := range rawConnections {
				_ = conn.Close()
			}
		}()

		// Test subscribe to channels
		channels := []string{"sub_channel1", "sub_channel2", "sub_channel3"}
		command := []resp.Value{resp.StringValue("SUBSCRIBE")}
		for _, channel := range channels {
			command = append(command, resp.StringValue(channel))
		}
		for _, conn := range connections {
			if err := conn.WriteArray(command); err != nil {
				t.Error(err)
				return
			}
			for i := 0; i < len(channels); i++ {
				// Read all the subscription confirmations from the connection.
				if _, _, err := conn.ReadValue(); err != nil {
					t.Error(err)
					return
				}
			}
		}
		activeChannels, err := mockServer.PubSubChannels("*")
		if err != nil {
			t.Error(err)
			return
		}
		numSubs, err := mockServer.PubSubNumSub(channels...)
		if err != nil {
			t.Error(err)
			return
		}
		for _, channel := range channels {
			// Check if the channel exists in the pubsub module.
			if !slices.Contains(activeChannels, channel) {
				t.Errorf("expected pubsub to contain channel \"%s\" but it was not found", channel)
				return
			}
			// Check if the channel has the right number of subscribers.
			if numSubs[channel] != len(connections) {
				t.Errorf("expected channel \"%s\" to have %d subscribers, got %d",
					channel, len(connections), numSubs[channel])
				return
			}
		}

		// Test subscribe to patterns
		patterns := []string{"psub_channel*"}
		command = []resp.Value{resp.StringValue("PSUBSCRIBE")}
		for _, pattern := range patterns {
			command = append(command, resp.StringValue(pattern))
		}
		for _, conn := range connections {
			if err := conn.WriteArray(command); err != nil {
				t.Error(err)
				return
			}
			for i := 0; i < len(patterns); i++ {
				// Read all the pattern subscription confirmations from the connection.
				if _, _, err := conn.ReadValue(); err != nil {
					t.Error(err)
					return
				}
			}
		}
		numSubs, err = mockServer.PubSubNumSub(patterns...)
		if err != nil {
			t.Error(err)
			return
		}
		for _, pattern := range patterns {
			activePatterns, err := mockServer.PubSubChannels(pattern)
			if err != nil {
				t.Error(err)
				return
			}
			// Check if pattern channel exists in pubsub module.
			if !slices.Contains(activePatterns, pattern) {
				t.Errorf("expected pubsub to contain pattern channel \"%s\" but it was not found", pattern)
				return
			}
			// Check if the channel has all the connections from above.
			if numSubs[pattern] != len(connections) {
				t.Errorf("expected pattern channel \"%s\" to have %d subscribers, got %d",
					pattern, len(connections), numSubs[pattern])
				return
			}
		}
	})

	t.Run("Test_HandleUnsubscribe", func(t *testing.T) {
		t.Parallel()

		var rawConnections []net.Conn
		generateConnections := func(noOfConnections int) []*resp.Conn {
			connections := make([]*resp.Conn, noOfConnections)
			for i := 0; i < noOfConnections; i++ {
				conn, err := internal.GetConnection("localhost", port)
				if err != nil {
					t.Error(err)
				}
				rawConnections = append(rawConnections, conn)
				connections[i] = resp.NewConn(conn)
			}
			return connections
		}
		defer func() {
			for _, conn := range rawConnections {
				_ = conn.Close()
			}
		}()

		verifyResponse := func(res resp.Value, expectedResponse [][]string) {
			v := res.Array()
			if len(v) != len(expectedResponse) {
				t.Errorf("expected subscribe response of length %d, but got %d", len(expectedResponse), len(v))
			}
			for _, item := range v {
				arr := item.Array()
				if len(arr) != 3 {
					t.Errorf("expected subscribe response item to be length %d, but got %d", 3, len(arr))
				}
				if !slices.ContainsFunc(expectedResponse, func(strings []string) bool {
					return strings[0] == arr[0].String() && strings[1] == arr[1].String() && strings[2] == arr[2].String()
				}) {
					t.Errorf("expected to find item \"%s\" in response, did not find it.", arr[1].String())
				}
			}
		}

		tests := []struct {
			subChannels       []string              // All channels to subscribe to
			subPatterns       []string              // All patterns to subscribe to
			unSubChannels     []string              // Channels to unsubscribe from
			unSubPatterns     []string              // Patterns to unsubscribe from
			remainChannels    []string              // Channels to remain subscribed to
			remainPatterns    []string              // Patterns to remain subscribed to
			targetConn        *resp.Conn            // Connection used to test unsubscribe functionality
			otherConnections  []*resp.Conn          // Connections to fill the subscribers list for channels and patterns
			expectedResponses map[string][][]string // The expected response from the handler
		}{
			{ // 1. Unsubscribe from channels and patterns
				subChannels:      []string{"xx_channel_one", "xx_channel_two", "xx_channel_three", "xx_channel_four"},
				subPatterns:      []string{"xx_pattern_[ab]", "xx_pattern_[cd]", "xx_pattern_[ef]", "xx_pattern_[gh]"},
				unSubChannels:    []string{"xx_channel_one", "xx_channel_two"},
				unSubPatterns:    []string{"xx_pattern_[ab]"},
				remainChannels:   []string{"xx_channel_three", "xx_channel_four"},
				remainPatterns:   []string{"xx_pattern_[cd]", "xx_pattern_[ef]", "xx_pattern_[gh]"},
				targetConn:       generateConnections(1)[0],
				otherConnections: generateConnections(20),
				expectedResponses: map[string][][]string{
					"channel": {
						{"unsubscribe", "xx_channel_one", "1"},
						{"unsubscribe", "xx_channel_two", "2"},
					},
					"pattern": {
						{"punsubscribe", "xx_pattern_[ab]", "1"},
					},
				},
			},
			{ // 2. Unsubscribe from all channels no channel or pattern is passed to command
				subChannels:      []string{"xx_channel_one", "xx_channel_two", "xx_channel_three", "xx_channel_four"},
				subPatterns:      []string{"xx_pattern_[ab]", "xx_pattern_[cd]", "xx_pattern_[ef]", "xx_pattern_[gh]"},
				unSubChannels:    []string{},
				unSubPatterns:    []string{},
				remainChannels:   []string{},
				remainPatterns:   []string{},
				targetConn:       generateConnections(1)[0],
				otherConnections: generateConnections(20),
				expectedResponses: map[string][][]string{
					"channel": {
						{"unsubscribe", "xx_channel_one", "1"},
						{"unsubscribe", "xx_channel_two", "2"},
						{"unsubscribe", "xx_channel_three", "3"},
						{"unsubscribe", "xx_channel_four", "4"},
					},
					"pattern": {
						{"punsubscribe", "xx_pattern_[ab]", "1"},
						{"punsubscribe", "xx_pattern_[cd]", "2"},
						{"punsubscribe", "xx_pattern_[ef]", "3"},
						{"punsubscribe", "xx_pattern_[gh]", "4"},
					},
				},
			},
			{ // 3. Don't unsubscribe from any channels or patterns if the provided ones are non-existent
				subChannels:      []string{"xx_channel_one", "xx_channel_two", "xx_channel_three", "xx_channel_four"},
				subPatterns:      []string{"xx_pattern_[ab]", "xx_pattern_[cd]", "xx_pattern_[ef]", "xx_pattern_[gh]"},
				unSubChannels:    []string{"xx_channel_non_existent_channel"},
				unSubPatterns:    []string{"xx_channel_non_existent_pattern_[ae]"},
				remainChannels:   []string{"xx_channel_one", "xx_channel_two", "xx_channel_three", "xx_channel_four"},
				remainPatterns:   []string{"xx_pattern_[ab]", "xx_pattern_[cd]", "xx_pattern_[ef]", "xx_pattern_[gh]"},
				targetConn:       generateConnections(1)[0],
				otherConnections: generateConnections(20),
				expectedResponses: map[string][][]string{
					"channel": {},
					"pattern": {},
				},
			},
		}

		for _, test := range tests {
			// Subscribe to channels.
			for _, conn := range append(test.otherConnections, test.targetConn) {
				command := []resp.Value{resp.StringValue("SUBSCRIBE")}
				for _, channel := range test.subChannels {
					command = append(command, resp.StringValue(channel))
				}
				if err := conn.WriteArray(command); err != nil {
					t.Error(err)
					return
				}
				for i := 0; i < len(test.subChannels); i++ {
					// Read channel subscription confirmations from connection.
					if _, _, err := conn.ReadValue(); err != nil {
						t.Error(err)
					}
				}

				// Subscribe to patterns.
				command = []resp.Value{resp.StringValue("PSUBSCRIBE")}
				for _, pattern := range test.subPatterns {
					command = append(command, resp.StringValue(pattern))
				}
				if err := conn.WriteArray(command); err != nil {
					t.Error(err)
					return
				}
				for i := 0; i < len(test.subPatterns); i++ {
					// Read pattern subscription confirmations from connection.
					if _, _, err := conn.ReadValue(); err != nil {
						t.Error(err)
					}
				}

			}

			// Unsubscribe the target connection from the unsub channels.
			command := []resp.Value{resp.StringValue("UNSUBSCRIBE")}
			for _, channel := range test.unSubChannels {
				command = append(command, resp.StringValue(channel))
			}
			if err := test.targetConn.WriteArray(command); err != nil {
				t.Error(err)
				return
			}
			res, _, err := test.targetConn.ReadValue()
			if err != nil {
				t.Error(err)
				return
			}
			verifyResponse(res, test.expectedResponses["channel"])

			// Unsubscribe the target connection from the unsub patterns.
			command = []resp.Value{resp.StringValue("PUNSUBSCRIBE")}
			for _, pattern := range test.unSubPatterns {
				command = append(command, resp.StringValue(pattern))
			}
			if err = test.targetConn.WriteArray(command); err != nil {
				t.Error(err)
				return
			}
			res, _, err = test.targetConn.ReadValue()
			if err != nil {
				t.Error(err)
				return
			}
			verifyResponse(res, test.expectedResponses["pattern"])
		}
	})

	t.Run("Test_HandlePublish", func(t *testing.T) {
		t.Parallel()

		var rawConnections []net.Conn
		establishConnection := func() *resp.Conn {
			conn, err := internal.GetConnection("localhost", port)
			if err != nil {
				t.Error(err)
			}
			rawConnections = append(rawConnections, conn)
			return resp.NewConn(conn)
		}
		defer func() {
			for _, conn := range rawConnections {
				_ = conn.Close()
			}
		}()

		// verifyChannelMessage reads the message from the connection and asserts whether
		// it's the message we expect to read as a subscriber of a channel or pattern.
		verifyEvent := func(client *resp.Conn, expected []string) {
			rv, _, err := client.ReadValue()
			if err != nil {
				t.Error(err)
			}
			v := rv.Array()
			for i := 0; i < len(v); i++ {
				if v[i].String() != expected[i] {
					t.Errorf("expected item at index %d to be \"%s\", got \"%s\"", i, expected[i], v[i].String())
				}
			}
		}

		// The subscribe function handles subscribing the connection to the given
		// channels and patterns and reading/verifying the message sent by the server after
		// subscription.
		subscribe := func(client *resp.Conn, channels []string, patterns []string) {
			// Subscribe to channels
			command := []resp.Value{resp.StringValue("SUBSCRIBE")}
			for _, channel := range channels {
				command = append(command, resp.StringValue(channel))
			}
			if err := client.WriteArray(command); err != nil {
				t.Error(err)
			}
			for i := 0; i < len(channels); i++ {
				// Read channel subscription confirmations.
				if _, _, err := client.ReadValue(); err != nil {
					t.Error(err)
				}
			}

			// Subscribe to all the patterns
			command = []resp.Value{resp.StringValue("PSUBSCRIBE")}
			for _, pattern := range patterns {
				command = append(command, resp.StringValue(pattern))
			}
			if err := client.WriteArray(command); err != nil {
				t.Error(err)
			}
			for i := 0; i < len(patterns); i++ {
				// Read pattern subscription confirmations.
				if _, _, err := client.ReadValue(); err != nil {
					t.Error(err)
				}
			}
		}

		subscriptions := []struct {
			client   *resp.Conn
			channels []string
			patterns []string
		}{
			{
				client:   establishConnection(),
				channels: []string{"pub_channel_1", "pub_channel_2", "pub_channel_3"},
				patterns: []string{"pub_channel_[456]"},
			},
			{
				client:   establishConnection(),
				channels: []string{"pub_channel_6", "pub_channel_7"},
				patterns: []string{"pub_channel_[891]"},
			},
		}
		for _, subscription := range subscriptions {
			// Subscribe to channels and patterns.
			subscribe(subscription.client, subscription.channels, subscription.patterns)
		}

		type Subscriber struct {
			client  *resp.Conn
			channel string
		}

		tests := []struct {
			channel     string
			message     string
			subscribers []Subscriber
		}{
			{
				channel: "pub_channel_1",
				message: "Test both subscribers 1",
				subscribers: []Subscriber{
					{client: subscriptions[0].client, channel: "pub_channel_1"},
					{client: subscriptions[1].client, channel: "pub_channel_[891]"},
				},
			},
			{
				channel: "pub_channel_6",
				message: "Test both subscribers 2",
				subscribers: []Subscriber{
					{client: subscriptions[0].client, channel: "pub_channel_[456]"},
					{client: subscriptions[1].client, channel: "pub_channel_6"},
				},
			},
			{
				channel: "pub_channel_2",
				message: "Test subscriber 1 1",
				subscribers: []Subscriber{
					{client: subscriptions[0].client, channel: "pub_channel_2"},
				},
			},
			{
				channel: "pub_channel_3",
				message: "Test subscriber 1 2",
				subscribers: []Subscriber{
					{client: subscriptions[0].client, channel: "pub_channel_3"},
				},
			},
			{
				channel: "pub_channel_4",
				message: "Test both subscribers 2",
				subscribers: []Subscriber{
					{client: subscriptions[0].client, channel: "pub_channel_[456]"},
				},
			},
			{
				channel: "pub_channel_5",
				message: "Test subscriber 1 3",
				subscribers: []Subscriber{
					{client: subscriptions[0].client, channel: "pub_channel_[456]"},
				},
			},
			{
				channel: "pub_channel_7",
				message: "Test subscriber 2 1",
				subscribers: []Subscriber{
					{client: subscriptions[1].client, channel: "pub_channel_7"},
				},
			},
			{
				channel: "pub_channel_8",
				message: "Test subscriber 2 2",
				subscribers: []Subscriber{
					{client: subscriptions[1].client, channel: "pub_channel_[891]"},
				},
			},
			{
				channel: "pub_channel_9",
				message: "Test subscriber 2 3",
				subscribers: []Subscriber{
					{client: subscriptions[1].client, channel: "pub_channel_[891]"},
				},
			},
		}

		// Dial echovault to make publisher connection
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		publisher := resp.NewConn(conn)

		for _, test := range tests {
			err = publisher.WriteArray([]resp.Value{
				resp.StringValue("PUBLISH"),
				resp.StringValue(test.channel),
				resp.StringValue(test.message),
			})
			if err != nil {
				t.Error(err)
			}

			rv, _, err := publisher.ReadValue()
			if err != nil {
				t.Error(err)
			}
			if rv.String() != "OK" {
				t.Errorf("Expected publish response to be \"OK\", got \"%s\"", rv.String())
			}

			for _, sub := range test.subscribers {
				verifyEvent(sub.client, []string{"message", sub.channel, test.message})
			}
		}
	})

	t.Run("Test_HandlePubSubChannels", func(t *testing.T) {
		t.Parallel()

		verifyExpectedResponse := func(res resp.Value, expected []string) {
			if len(res.Array()) != len(expected) {
				t.Errorf("expected response array of length %d, got %d", len(expected), len(res.Array()))
			}
			for _, e := range expected {
				if !slices.ContainsFunc(res.Array(), func(v resp.Value) bool {
					return e == v.String()
				}) {
					t.Errorf("expected to find element \"%s\" in response array, could not find it", e)
				}
			}
		}

		port, err := internal.GetFreePort()
		if err != nil {
			t.Error(err)
			return
		}
		mockServer, err := setUpServer(port)
		if err != nil {
			t.Error(err)
			return
		}

		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			wg.Done()
			mockServer.Start()
		}()
		wg.Wait()

		subscribers := make([]*resp.Conn, 2)
		for i := 0; i < len(subscribers); i++ {
			conn, err := internal.GetConnection("localhost", port)
			if err != nil {
				t.Error(err)
				return
			}
			subscribers[i] = resp.NewConn(conn)
		}

		channels := []string{"channel_1", "channel_2", "channel_3"}
		patterns := []string{"channel_[123]", "channel_[456]"}

		subscriptions := []struct {
			client   *resp.Conn
			action   string
			channels []string
			patterns []string
		}{
			{
				client:   subscribers[0],
				action:   "SUBSCRIBE",
				channels: channels,
				patterns: make([]string, 0),
			},
			{
				client:   subscribers[1],
				action:   "PSUBSCRIBE",
				channels: make([]string, 0),
				patterns: patterns,
			},
		}
		for _, subscription := range subscriptions {
			command := []resp.Value{resp.StringValue(subscription.action)}
			if len(subscription.channels) > 0 {
				for _, channel := range subscription.channels {
					command = append(command, resp.StringValue(channel))
				}
			} else if len(subscription.patterns) > 0 {
				for _, pattern := range subscription.patterns {
					command = append(command, resp.StringValue(pattern))
				}
			}
			if err := subscription.client.WriteArray(command); err != nil {
				t.Error(err)
			}
			if len(subscription.channels) > 0 {
				for i := 0; i < len(subscription.channels); i++ {
					_, _, _ = subscription.client.ReadValue()
				}
				return
			}
			for i := 0; i < len(subscription.patterns); i++ {
				_, _, _ = subscription.client.ReadValue()
			}
		}

		// Get fresh connection for the next phase.
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		client := resp.NewConn(conn)

		// Check if all subscriptions are returned.
		if err = client.WriteArray([]resp.Value{resp.StringValue("PUBSUB"), resp.StringValue("CHANNELS")}); err != nil {
			t.Error(err)
		}
		res, _, err := client.ReadValue()
		if err != nil {
			t.Error(err)
		}
		verifyExpectedResponse(res, append(channels, patterns...))

		// Unsubscribe from one pattern and one channel before checking against a new slice of
		// expected channels/patterns in the response of the "PUBSUB CHANNELS" command.
		for _, unsubscribe := range []struct {
			client  *resp.Conn
			command []resp.Value
		}{
			{
				client:  subscribers[0],
				command: []resp.Value{resp.StringValue("UNSUBSCRIBE"), resp.StringValue("channel_2"), resp.StringValue("channel_3")},
			},
			{
				client:  subscribers[1],
				command: []resp.Value{resp.StringValue("UNSUBSCRIBE"), resp.StringValue("channel_[456]")},
			},
		} {
			if err = unsubscribe.client.WriteArray(unsubscribe.command); err != nil {
				t.Error(err)
			}
			for i := 0; i < len(unsubscribe.command[1:]); i++ {
				_, _, err = unsubscribe.client.ReadValue()
				if err != nil {
					t.Error(err)
				}
			}
		}

		// Return all the remaining channels.
		if err = client.WriteArray([]resp.Value{resp.StringValue("PUBSUB"), resp.StringValue("CHANNELS")}); err != nil {
			t.Error(err)
		}
		res, _, err = client.ReadValue()
		if err != nil {
			t.Error(err)
		}
		verifyExpectedResponse(res, []string{"channel_1", "channel_[123]"})

		// Return only one of the remaining channels when passed a pattern that matches it.
		if err = client.WriteArray([]resp.Value{
			resp.StringValue("PUBSUB"),
			resp.StringValue("CHANNELS"),
			resp.StringValue("channel_[189]"),
		}); err != nil {
			t.Error(err)
		}
		verifyExpectedResponse(res, []string{"channel_1"})

		// Return both remaining channels when passed a pattern that matches them.
		if err := client.WriteArray([]resp.Value{
			resp.StringValue("PUBSUB"),
			resp.StringValue("CHANNELS"),
			resp.StringValue("channel_[123]"),
		}); err != nil {
			t.Error(err)
		}
		res, _, err = client.ReadValue()
		if err != nil {
			t.Error(err)
		}
		verifyExpectedResponse(res, []string{"channel_1", "channel_[123]"})

		// Return no channels when passed a pattern that does not match either channel.
		if err = client.WriteArray([]resp.Value{
			resp.StringValue("PUBSUB"),
			resp.StringValue("CHANNELS"),
			resp.StringValue("channel_[456]"),
		}); err != nil {
			t.Error(err)
		}
		res, _, err = client.ReadValue()
		if err != nil {
			t.Error(err)
		}
		verifyExpectedResponse(res, []string{})
	})

	t.Run("Test_HandleNumPat", func(t *testing.T) {
		t.Parallel()

		port, err := internal.GetFreePort()
		if err != nil {
			t.Error(err)
			return
		}

		mockServer, err := setUpServer(port)
		if err != nil {
			t.Error(err)
			return
		}

		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			wg.Done()
			mockServer.Start()
		}()
		wg.Wait()

		// Create subscribers.
		subscribers := make([]*resp.Conn, 3)
		for i := 0; i < len(subscribers); i++ {
			conn, err := internal.GetConnection("localhost", port)
			if err != nil {
				t.Error(err)
				return
			}
			subscribers[i] = resp.NewConn(conn)
		}

		patterns := []string{"pattern_[123]", "pattern_[456]", "pattern_[789]"}

		// Subscribe to all patterns
		for _, client := range subscribers {
			command := []resp.Value{resp.StringValue("PSUBSCRIBE")}
			for _, pattern := range patterns {
				command = append(command, resp.StringValue(pattern))
			}
			if err := client.WriteArray(command); err != nil {
				t.Error(err)
			}
			// Read subscription responses to make sure we've subscribed to all the channels.
			for i := 0; i < len(patterns); i++ {
				res, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
				}
				if len(res.Array()) != 3 {
					t.Errorf("expected array response of length %d, got %d", 3, len(res.Array()))
				}
				if !strings.EqualFold(res.Array()[0].String(), "psubscribe") {
					t.Errorf("expected the first array item to be \"psubscribe\", got \"%s\"", res.Array()[0].String())
				}
				if !slices.Contains(patterns, res.Array()[1].String()) {
					t.Errorf("unexpected channel name \"%s\", expected %v", res.Array()[1].String(), patterns)
				}
			}
		}

		// Get fresh connection for the next phase.
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		client := resp.NewConn(conn)

		// Check that we receive all the patterns with NUMPAT commands.
		if err = client.WriteArray([]resp.Value{resp.StringValue("PUBSUB"), resp.StringValue("NUMPAT")}); err != nil {
			t.Error(err)
		}
		res, _, err := client.ReadValue()
		if res.Integer() != len(patterns) {
			t.Errorf("expected response \"%d\", got \"%d\"", len(patterns), res.Integer())
		}

		// Unsubscribe all subscribers from one pattern and check if the response is updated.
		for _, subscriber := range subscribers {
			if err = subscriber.WriteArray([]resp.Value{
				resp.StringValue("PUNSUBSCRIBE"),
				resp.StringValue(patterns[0]),
			}); err != nil {
				t.Error(err)
			}
			res, _, err = subscriber.ReadValue()
			if err != nil {
				t.Error(err)
			}
			if len(res.Array()[0].Array()) != 3 {
				t.Errorf("expected array response of length %d, got %d", 3, len(res.Array()[0].Array()))
			}
			if !strings.EqualFold(res.Array()[0].Array()[0].String(), "punsubscribe") {
				t.Errorf("expected the first array item to be \"punsubscribe\", got \"%s\"", res.Array()[0].Array()[0].String())
			}
			if res.Array()[0].Array()[1].String() != patterns[0] {
				t.Errorf("unexpected channel name \"%s\", expected %s", res.Array()[0].Array()[1].String(), patterns[0])
			}
		}
		if err = client.WriteArray([]resp.Value{resp.StringValue("PUBSUB"), resp.StringValue("NUMPAT")}); err != nil {
			t.Error(err)
		}
		res, _, err = client.ReadValue()
		if res.Integer() != len(patterns)-1 {
			t.Errorf("expected response \"%d\", got \"%d\"", len(patterns)-1, res.Integer())
		}

		// Unsubscribe from all the channels and check if we get a 0 response
		for _, subscriber := range subscribers {
			for _, pattern := range patterns[1:] {
				if err = subscriber.WriteArray([]resp.Value{
					resp.StringValue("PUNSUBSCRIBE"),
					resp.StringValue(pattern),
				}); err != nil {
					t.Error(err)
				}
				res, _, err = subscriber.ReadValue()
				if err != nil {
					t.Error(err)
				}
				if len(res.Array()[0].Array()) != 3 {
					t.Errorf("expected array response of length %d, got %d", 3,
						len(res.Array()[0].Array()))
				}
				if !strings.EqualFold(res.Array()[0].Array()[0].String(), "punsubscribe") {
					t.Errorf("expected the first array item to be \"punsubscribe\", got \"%s\"",
						res.Array()[0].Array()[0].String())
				}
				if res.Array()[0].Array()[1].String() != pattern {
					t.Errorf("unexpected channel name \"%s\", expected %s",
						res.Array()[0].Array()[1].String(), pattern)
				}
			}
		}
		if err = client.WriteArray([]resp.Value{resp.StringValue("PUBSUB"), resp.StringValue("NUMPAT")}); err != nil {
			t.Error(err)
		}
		res, _, err = client.ReadValue()
		if res.Integer() != 0 {
			t.Errorf("expected response \"%d\", got \"%d\"", 0, res.Integer())
		}
	})

	t.Run("Test_HandleNumSub", func(t *testing.T) {
		t.Parallel()

		port, err := internal.GetFreePort()
		if err != nil {
			t.Error(err)
			return
		}

		mockServer, err := setUpServer(port)
		if err != nil {
			t.Error(err)
			return
		}

		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			wg.Done()
			mockServer.Start()
		}()
		wg.Wait()

		channels := []string{"channel_1", "channel_2", "channel_3"}

		for i := 0; i < 3; i++ {
			conn, err := internal.GetConnection("localhost", port)
			if err != nil {
				t.Error(err)
				return
			}
			client := resp.NewConn(conn)
			command := []resp.Value{
				resp.StringValue("SUBSCRIBE"),
			}
			for _, channel := range channels {
				command = append(command, resp.StringValue(channel))
			}
			err = client.WriteArray(command)
			if err != nil {
				t.Error(err)
			}

			// Read subscription responses to make sure we've subscribed to all the channels.
			for i := 0; i < len(channels); i++ {
				res, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
				}
				if len(res.Array()) != 3 {
					t.Errorf("expected array response of length %d, got %d", 3, len(res.Array()))
				}
				if !strings.EqualFold(res.Array()[0].String(), "subscribe") {
					t.Errorf("expected the first array item to be \"subscribe\", got \"%s\"", res.Array()[0].String())
				}
				if !slices.Contains(channels, res.Array()[1].String()) {
					t.Errorf("unexpected channel name \"%s\", expected %v", res.Array()[1].String(), channels)
				}
			}
		}

		// Get fresh connection for the next phase.
		conn, err := internal.GetConnection("localhost", port)
		if err != nil {
			t.Error(err)
			return
		}
		client := resp.NewConn(conn)

		tests := []struct {
			name             string
			cmd              []string
			expectedResponse [][]string
		}{
			{
				name:             "1. Get all subscriptions on existing channels",
				cmd:              append([]string{"PUBSUB", "NUMSUB"}, channels...),
				expectedResponse: [][]string{{"channel_1", "3"}, {"channel_2", "3"}, {"channel_3", "3"}},
			},
			{
				name: "2. Get all the subscriptions of on existing channels and a few non-existent ones",
				cmd:  append([]string{"PUBSUB", "NUMSUB", "non_existent_channel_1", "non_existent_channel_2"}, channels...),
				expectedResponse: [][]string{
					{"non_existent_channel_1", "0"},
					{"non_existent_channel_2", "0"},
					{"channel_1", "3"},
					{"channel_2", "3"},
					{"channel_3", "3"},
				},
			},
			{
				name:             "3. Get an empty array when channels are not provided in the command",
				cmd:              []string{"PUBSUB", "NUMSUB"},
				expectedResponse: make([][]string, 0),
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				var command []resp.Value
				for _, token := range test.cmd {
					command = append(command, resp.StringValue(token))
				}

				if err = client.WriteArray(command); err != nil {
					t.Error(err)
				}

				res, _, err := client.ReadValue()
				if err != nil {
					t.Error(err)
				}

				arr := res.Array()
				if len(arr) != len(test.expectedResponse) {
					t.Errorf("expected response array of length %d, got %d", len(test.expectedResponse), len(arr))
				}

				for _, item := range arr {
					itemArr := item.Array()
					if len(itemArr) != 2 {
						t.Errorf("expected each response item to be of length 2, got %d", len(itemArr))
					}
					if !slices.ContainsFunc(test.expectedResponse, func(expected []string) bool {
						return expected[0] == itemArr[0].String() && expected[1] == itemArr[1].String()
					}) {
						t.Errorf("could not find entry with channel \"%s\", with %d subscribers in expected response",
							itemArr[0].String(), itemArr[1].Integer())
					}
				}
			})
		}
	})
}
