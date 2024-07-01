package echovault

import (
	"context"
	"github.com/echovault/echovault/internal"
	"github.com/echovault/echovault/internal/config"
	"github.com/echovault/echovault/internal/constants"
)

func createEchoVault() *EchoVault {
	ev, _ := NewEchoVault(
		WithConfig(config.Config{
			DataDir:        "",
			EvictionPolicy: constants.NoEviction,
		}),
	)
	return ev
}

func createEchoVaultWithConfig(conf config.Config) *EchoVault {
	ev, _ := NewEchoVault(
		WithConfig(conf),
	)
	return ev
}

func presetValue(server *EchoVault, ctx context.Context, key string, value interface{}) error {
	ctx = context.WithValue(ctx, "Database", 0)
	if err := server.setValues(ctx, map[string]interface{}{key: value}); err != nil {
		return err
	}
	return nil
}

func presetKeyData(server *EchoVault, ctx context.Context, key string, data internal.KeyData) {
	ctx = context.WithValue(ctx, "Database", 0)
	_ = server.setValues(ctx, map[string]interface{}{key: data.Value})
	server.setExpiry(ctx, key, data.ExpireAt, false)
}
