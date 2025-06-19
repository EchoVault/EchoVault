package sugardb

import (
	"context"
	"strconv"

	"github.com/echovault/sugardb/internal"
	"github.com/echovault/sugardb/internal/constants"
	"github.com/echovault/sugardb/pkg/config"
)

func createSugarDB() *SugarDB {
	ev, _ := NewSugarDB(
		WithConfig(config.Config{
			DataDir:        "",
			EvictionPolicy: constants.NoEviction,
		}),
	)
	return ev
}

func createSugarDBWithConfig(conf config.Config) *SugarDB {
	ev, _ := NewSugarDB(
		WithConfig(conf),
	)
	return ev
}

func presetValue(server *SugarDB, ctx context.Context, key string, value interface{}) error {
	ctx = context.WithValue(ctx, "Database", 0)
	if err := server.setValues(ctx, map[string]interface{}{key: value}); err != nil {
		return err
	}
	return nil
}

func presetKeyData(server *SugarDB, ctx context.Context, key string, data internal.KeyData) {
	ctx = context.WithValue(ctx, "Database", 0)
	_ = server.setValues(ctx, map[string]interface{}{key: data.Value})
	server.setExpiry(ctx, key, data.ExpireAt, false)
}

func getValue(server *SugarDB, ctx context.Context, key string, database string) (interface{}, error) {
	db, err := strconv.Atoi(database)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, "Database", db)

	return server.getValues(ctx, []string{key})[key], err
}
