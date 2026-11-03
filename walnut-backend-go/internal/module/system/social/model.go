// Package social 对应 sys_social（社交授权关系表）。
package social

import "walnut-seed-go/internal/pkg/modelx"

// SysSocial 社交授权关系表 sys_social。
//
// ⚠️ 含 del_flag：查询一律过滤 '0'，删除为逻辑删除（本模块仅暴露 /list）。
type SysSocial struct {
	modelx.BaseModel

	UserID           int64   `gorm:"column:user_id;not null"`
	AuthID           string  `gorm:"column:auth_id;not null"`
	Source           string  `gorm:"column:source;not null"`
	OpenID           *string `gorm:"column:open_id"`
	UserName         string  `gorm:"column:user_name;not null"`
	NickName         *string `gorm:"column:nick_name"`
	Email            *string `gorm:"column:email"`
	Avatar           *string `gorm:"column:avatar"`
	AccessToken      string  `gorm:"column:access_token;not null"`
	ExpireIn         *int    `gorm:"column:expire_in"`
	RefreshToken     *string `gorm:"column:refresh_token"`
	AccessCode       *string `gorm:"column:access_code"`
	UnionID          *string `gorm:"column:union_id"`
	Scope            *string `gorm:"column:scope"`
	TokenType        *string `gorm:"column:token_type"`
	IDToken          *string `gorm:"column:id_token"`
	MacAlgorithm     *string `gorm:"column:mac_algorithm"`
	MacKey           *string `gorm:"column:mac_key"`
	Code             *string `gorm:"column:code"`
	OauthToken       *string `gorm:"column:oauth_token"`
	OauthTokenSecret *string `gorm:"column:oauth_token_secret"`
	DelFlag          string  `gorm:"column:del_flag;not null"`
}

func (SysSocial) TableName() string { return "sys_social" }
