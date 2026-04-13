package golake

import (
	"context"
	"database/sql"
	"database/sql/driver"
)

// LakeDriver is a context of Go Driver
type LakeDriver struct{}

// Open creates a new connection.
func (d LakeDriver) Open(dsn string) (driver.Conn, error) {
	logger.Info("Open")
	ctx := context.Background()
	cfg, err := ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	return d.OpenWithConfig(ctx, cfg)
}

func (d LakeDriver) OpenConnector(dsn string) (driver.Connector, error) {
	return ParseDSN(dsn)
}

// OpenWithConfig creates a new connection with the given Config.
func (d LakeDriver) OpenWithConfig(
	ctx context.Context,
	config *Config,
) (driver.Conn, error) {
	logger.Info("OpenWithConfig")
	dc, err := buildLakeConn(ctx, config)
	if err != nil {
		return nil, err
	}
	return dc, nil
}

var logger = CreateDefaultLogger()

func init() {
	sql.Register("databend", LakeDriver{})
	sql.Register("lake", LakeDriver{})
	_ = logger.SetLogLevel("error")
}
