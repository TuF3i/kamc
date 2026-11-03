// Package geoip 可选的 IP 地理信息解析：加载 MaxMind GeoLite2/GeoIP2 数据库
// （AM_GEOIP_DB 指向 .mmdb 文件），为审计事件补充国家/地区/城市。
//
// 未配置或加载失败时返回 nil，调用方按 no-op 处理（审计不因 GeoIP 降级失败）。
// maxminddb.Reader 并发安全，可直接多 goroutine 查询。
package geoip

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"github.com/oschwald/maxminddb-golang"
)

// Location 一次查询结果（空字段 = 数据库无该维度数据）
type Location struct {
	Country string // ISO 代码，如 CN / US
	Region  string // 一级行政区
	City    string
}

// Resolver GeoIP 解析器（nil 安全：nil 接收者 Lookup 返回空）
type Resolver struct {
	db *maxminddb.Reader
}

// New 打开数据库文件（路径为空返回 nil, nil = 未启用）。
func New(path string) (*Resolver, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	db, err := maxminddb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("geoip -> open %s: %w", path, err)
	}
	log.Printf("[geoip] database loaded: %s", path)
	return &Resolver{db: db}, nil
}

// FromEnv 按 AM_GEOIP_DB 装配（未配置返回 nil；配置但加载失败直接报错，
// 由装配层决定降级——默认降级为无 GeoIP 并打警告）。
func FromEnv() *Resolver {
	path := os.Getenv("AM_GEOIP_DB")
	if path == "" {
		return nil
	}
	r, err := New(path)
	if err != nil {
		log.Printf("[geoip] WARNING: %v（审计将不含地理信息）", err)
		return nil
	}
	return r
}

// Close 释放数据库（幂等）
func (r *Resolver) Close() {
	if r == nil || r.db == nil {
		return
	}
	_ = r.db.Close()
}

// Lookup 查询 IP 的地理信息（非法 IP/未命中返回零值）
func (r *Resolver) Lookup(ip string) Location {
	if r == nil || r.db == nil || ip == "" {
		return Location{}
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return Location{}
	}
	var rec struct {
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
		Subdivisions []struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"subdivisions"`
		City struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"city"`
	}
	if err := r.db.Lookup(parsed, &rec); err != nil {
		return Location{}
	}
	loc := Location{Country: rec.Country.ISOCode}
	if len(rec.Subdivisions) > 0 {
		loc.Region = rec.Subdivisions[0].Names["en"]
	}
	loc.City = rec.City.Names["en"]
	return loc
}
