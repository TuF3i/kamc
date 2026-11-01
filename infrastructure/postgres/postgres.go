// Package postgres PostgreSQL（gorm）连接的统一构建入口。
//
// 平台各模块经此包获取数据库连接：模块的依赖"注入优先，否则自建"——
// 未注入的模块在此自建连接，自建的连接由该模块自己的 ReleaseRuntime 负责关闭
// （ownDB 所有权约定，见 AGENT.md）。
package postgres

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DefaultDSN 未配置 DATABASE_URL 时的本地开发兜底值
const DefaultDSN = "host=127.0.0.1 port=5432 user=postgres password=postgres dbname=kamc sslmode=disable TimeZone=Asia/Shanghai"

// Option 连接选项
type Option func(*options)

type options struct {
	dsn         string
	maxOpen     int
	maxIdle     int
	maxLifetime time.Duration
}

// WithDSN 指定连接串（默认读环境变量 DATABASE_URL，未设置再用 DefaultDSN）
func WithDSN(dsn string) Option {
	return func(o *options) { o.dsn = dsn }
}

// WithPool 覆盖连接池参数（默认 max_open=20 / max_idle=5 / lifetime=1h）
func WithPool(maxOpen, maxIdle int, maxLifetime time.Duration) Option {
	return func(o *options) {
		o.maxOpen, o.maxIdle, o.maxLifetime = maxOpen, maxIdle, maxLifetime
	}
}

// NewPostgresClient 构建 gorm 连接：SQL 日志仅 Warn 起记，避免框架 SQL 刷屏。
func NewPostgresClient(opts ...Option) (*gorm.DB, error) {
	o := &options{maxOpen: 20, maxIdle: 5, maxLifetime: time.Hour}
	for _, opt := range opts {
		opt(o)
	}

	dsn := o.dsn
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		dsn = DefaultDSN
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("postgres -> open: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("postgres -> unwrap sql db: %w", err)
	}
	sqlDB.SetMaxOpenConns(o.maxOpen)
	sqlDB.SetMaxIdleConns(o.maxIdle)
	sqlDB.SetConnMaxLifetime(o.maxLifetime)

	log.Printf("[postgres] connected (max_open=%d max_idle=%d)", o.maxOpen, o.maxIdle)
	return db, nil
}
