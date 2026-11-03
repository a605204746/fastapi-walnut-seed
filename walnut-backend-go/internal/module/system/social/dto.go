// 本文件定义社交授权关系的出参 DTO，对应 Python 端 module_system/social/schema.py 的 OutSchema。
//
// ⚠️ 键序 = SocialOutSchema 声明序（alias_generator=to_camel）；本模块仅 /list 用到。

package social

import (
	"walnut-seed-go/internal/pkg/idtype"
	"walnut-seed-go/internal/pkg/timex"
)

// Out 社交授权关系出参。
type Out struct {
	ID               *idtype.JSInt   `json:"id"`
	UserID           *idtype.JSInt   `json:"userId"`
	AuthID           *string         `json:"authId"`
	Source           *string         `json:"source"`
	AccessToken      *string         `json:"accessToken"`
	ExpireIn         *idtype.JSInt   `json:"expireIn"`
	RefreshToken     *string         `json:"refreshToken"`
	OpenID           *string         `json:"openId"`
	UserName         *string         `json:"userName"`
	NickName         *string         `json:"nickName"`
	Email            *string         `json:"email"`
	Avatar           *string         `json:"avatar"`
	AccessCode       *string         `json:"accessCode"`
	UnionID          *string         `json:"unionId"`
	Scope            *string         `json:"scope"`
	TokenType        *string         `json:"tokenType"`
	IDToken          *string         `json:"idToken"`
	MacAlgorithm     *string         `json:"macAlgorithm"`
	MacKey           *string         `json:"macKey"`
	Code             *string         `json:"code"`
	OauthToken       *string         `json:"oauthToken"`
	OauthTokenSecret *string         `json:"oauthTokenSecret"`
	CreateTime       *timex.JSONTime `json:"createTime"`
}

// NewOut 由实体构造出参。
func NewOut(r *SysSocial) Out {
	out := Out{
		ID:               jsIntPtr(r.ID),
		UserID:           jsIntPtr(r.UserID),
		AuthID:           strPtr(r.AuthID),
		Source:           strPtr(r.Source),
		AccessToken:      strPtr(r.AccessToken),
		ExpireIn:         intPtrToJSInt(r.ExpireIn),
		RefreshToken:     r.RefreshToken,
		OpenID:           r.OpenID,
		UserName:         strPtr(r.UserName),
		NickName:         r.NickName,
		Email:            r.Email,
		Avatar:           r.Avatar,
		AccessCode:       r.AccessCode,
		UnionID:          r.UnionID,
		Scope:            r.Scope,
		TokenType:        r.TokenType,
		IDToken:          r.IDToken,
		MacAlgorithm:     r.MacAlgorithm,
		MacKey:           r.MacKey,
		Code:             r.Code,
		OauthToken:       r.OauthToken,
		OauthTokenSecret: r.OauthTokenSecret,
	}
	if r.CreateTime != nil {
		jt := timex.FromTime(*r.CreateTime)
		out.CreateTime = &jt
	}
	return out
}

func jsIntPtr(v int64) *idtype.JSInt {
	j := idtype.JSInt(v)
	return &j
}

func intPtrToJSInt(v *int) *idtype.JSInt {
	if v == nil {
		return nil
	}
	j := idtype.JSInt(int64(*v))
	return &j
}

func strPtr(v string) *string { return &v }
