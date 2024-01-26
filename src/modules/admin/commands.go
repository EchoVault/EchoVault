package admin

import (
	"context"
	"errors"
	"github.com/echovault/echovault/src/utils"
	"net"
)

type Plugin struct {
	name        string
	commands    []utils.Command
	description string
}

func (p Plugin) Name() string {
	return p.name
}

func (p Plugin) Commands() []utils.Command {
	return p.commands
}

func (p Plugin) Description() string {
	return p.description
}

func NewModule() Plugin {
	return Plugin{
		name:        "AdminCommands",
		description: "Handle admin/server management commands",
		commands: []utils.Command{
			{
				Command:     "bgsave",
				Categories:  []string{utils.AdminCategory, utils.SlowCategory, utils.DangerousCategory},
				Description: "(BGSAVE) Trigger a snapshot save",
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					return []string{}, nil
				},
				HandlerFunc: func(ctx context.Context, cmd []string, server utils.Server, conn *net.Conn) ([]byte, error) {
					if err := server.TakeSnapshot(); err != nil {
						return nil, err
					}
					return []byte(utils.OK_RESPONSE), nil
				},
			},
			{
				Command:     "lastsave",
				Categories:  []string{utils.AdminCategory, utils.FastCategory, utils.DangerousCategory},
				Description: "(LASTSAVE) Get timestamp for the latest snapshot",
				Sync:        false,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					return []string{}, nil
				},
				HandlerFunc: func(ctx context.Context, cmd []string, server utils.Server, conn *net.Conn) ([]byte, error) {
					return nil, errors.New("LASTSAVE command not implemented")
				},
			},
			{
				Command:     "bgrewriteaof",
				Categories:  []string{utils.AdminCategory, utils.SlowCategory, utils.DangerousCategory},
				Description: "(BGREWRITEAOF) Trigger re-writing of append process",
				Sync:        false,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					return []string{}, nil
				},
				HandlerFunc: func(ctx context.Context, cmd []string, server utils.Server, conn *net.Conn) ([]byte, error) {
					return nil, errors.New("BGREWRITEAOF command not implemented")
				},
			},
		},
	}
}
