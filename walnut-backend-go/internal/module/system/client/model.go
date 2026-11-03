// Package client 对应 sys_client（系统授权表）。
//
// 登录时按 clientid 请求头查此表，校验 grant_type 是否被允许、
// 并取出 timeout / active_timeout 决定 JWT 与会话的有效期。
package client

import "walnut-seed-go/internal/pkg/modelx"

// SysClient 系统授权表 sys_client。
type SysClient struct {
	modelx.BaseModel

	// ClientID 客户端 id（前端 VITE_GLOB_APP_CLIENT_ID 与之对应）
	ClientID *string `gorm:"column:client_id"`
	// ClientKey 客户端 key（如 pc / app），写入在线用户信息
	ClientKey    *string `gorm:"column:client_key"`
	ClientSecret *string `gorm:"column:client_secret"`
	// GrantType 允许的授权类型，**逗号分隔**（如 "password,social"）
	GrantType  *string `gorm:"column:grant_type"`
	DeviceType *string `gorm:"column:device_type"`
	// ActiveTimeout token 活跃超时（秒），决定 Redis 会话 TTL（滑动过期）
	ActiveTimeout *int `gorm:"column:active_timeout"`
	// Timeout token 固定超时（秒），决定 JWT 的 exp（硬过期）
	Timeout *int    `gorm:"column:timeout"`
	Status  *string `gorm:"column:status"`
	DelFlag string  `gorm:"column:del_flag;not null"`
}

func (SysClient) TableName() string { return "sys_client" }
