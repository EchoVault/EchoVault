package connection

import (
	"context"
	"errors"
	"fmt"
	"github.com/echovault/echovault/src/utils"
	"net"
)

func handlePing(ctx context.Context, cmd []string, server utils.Server, conn *net.Conn) ([]byte, error) {
	switch len(cmd) {
	default:
		return nil, errors.New(utils.WrongArgsResponse)
	case 1:
		return []byte("+PONG\r\n"), nil
	case 2:
		return []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(cmd[1]), cmd[1])), nil
	}
}

func Commands() []utils.Command {
	return []utils.Command{
		{
			Command:     "connection",
			Categories:  []string{utils.FastCategory, utils.ConnectionCategory},
			Description: "(PING [value]) Ping the server. If a value is provided, the value will be echoed.",
			Sync:        false,
			KeyExtractionFunc: func(cmd []string) ([]string, error) {
				return []string{}, nil
			},
			HandlerFunc: handlePing,
		},
	}
}