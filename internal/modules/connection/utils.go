package connection

import (
	"fmt"
	"github.com/echovault/echovault/internal"
	"github.com/echovault/echovault/internal/constants"
	"strings"
)

type helloOptions struct {
	protocol   int
	clientname string
	auth       struct {
		authenticate bool
		username     string
		password     string
	}
}

func getHelloOptions(cmd []string, options helloOptions) (helloOptions, error) {
	if len(cmd) == 0 {
		return options, nil
	}
	switch strings.ToLower(cmd[0]) {
	case "auth":
		if len(cmd) < 3 {
			return options, fmt.Errorf(constants.WrongArgsResponse)
		}
		options.auth.authenticate = true
		options.auth.username = cmd[1]
		options.auth.password = cmd[2]
		return getHelloOptions(cmd[3:], options)
	case "setname":
		if len(cmd) < 2 {
			return options, fmt.Errorf(constants.WrongArgsResponse)
		}
		options.clientname = cmd[1]
		return getHelloOptions(cmd[2:], options)
	default:
		return options, fmt.Errorf("unknown keywork %s", strings.ToUpper(cmd[0]))
	}
}

func BuildHelloResponse(serverInfo internal.ServerInfo, connectionInfo internal.ConnectionInfo) []byte {
	var res []byte

	if connectionInfo.Protocol == 2 {
		// Construct RESP2 response.
		res = []byte("*14\r\n")
	} else {
		// Construct RESP3 response.
		res = []byte("%7\r\n")
	}

	res = append(res, []byte(fmt.Sprintf("+server\r\n$%d\r\n%s\r\n", len(serverInfo.Server), serverInfo.Server))...)
	res = append(res, []byte(fmt.Sprintf("+version\r\n$%d\r\n%s\r\n", len(serverInfo.Version), serverInfo.Version))...)
	res = append(res, []byte(fmt.Sprintf("+proto\r\n:%d\r\n", connectionInfo.Protocol))...)
	res = append(res, []byte(fmt.Sprintf("+id\r\n:%d\r\n", connectionInfo.Id))...)
	res = append(res, []byte(fmt.Sprintf("+mode\r\n$%d\r\n%s\r\n", len(serverInfo.Mode), serverInfo.Mode))...)
	res = append(res, []byte(fmt.Sprintf("+role\r\n$%d\r\n%s\r\n", len(serverInfo.Role), serverInfo.Role))...)
	res = append(res, []byte(fmt.Sprintf("+modules\r\n*%d\r\n", len(serverInfo.Modules)))...)
	for _, module := range serverInfo.Modules {
		res = append(res, []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(module), module))...)
	}
	return res
}
