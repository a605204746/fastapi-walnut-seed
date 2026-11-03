// Package config 对应 sys_config（参数配置表）。
package config

import "walnut-seed-go/internal/pkg/modelx"

// SysConfig 参数配置表 sys_config。
//
// P2 用到 ConfigKey = "sys.account.registerUser" 判断注册开关。
type SysConfig struct {
	modelx.BaseModel

	ConfigName  *string `gorm:"column:config_name"`
	ConfigKey   *string `gorm:"column:config_key"`
	ConfigValue *string `gorm:"column:config_value"`
	// ConfigType 系统内置：'Y' 是、'N' 否
	ConfigType *string `gorm:"column:config_type"`
	Remark     *string `gorm:"column:remark"`
}

func (SysConfig) TableName() string { return "sys_config" }

// 常用配置键，对应 Python 端硬编码的字符串常量。
const (
	// KeyRegisterUser 账号自助注册开关，值为 "true"/"false"
	KeyRegisterUser = "sys.account.registerUser"
	// KeyUserInitPassword 用户导入时的初始密码
	KeyUserInitPassword = "sys.user.initPassword"
)
