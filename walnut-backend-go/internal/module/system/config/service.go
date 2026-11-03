// 本文件实现参数配置域的业务逻辑，对应 Python 端 module_system/config/service.py。
//
// # 缓存语义（逐条镜像）
//
//   - 键 `sys_config:{config_key}`，值是**裸值字符串**（不是 JSON 信封）；
//   - 读时兼容「带引号的 JSON 字符串」形态（历史写入），能解包就解包；
//   - 写后回填缓存；改键名时删旧键；updateByKey 无 id 走批量 update + 回填；
//   - 缓存值写 None 在 Python 端会被 redis-py 拒绝并静默记日志 → Go 侧 nil 时**跳过写入**。

package config

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/cachex"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
)

// nowFunc 提供「当前时间」，便于单测钉住审计字段。
var nowFunc = time.Now

// 缓存键前缀，对应 CacheNames.SYS_CONFIG_KEY。
const cacheConfigKey = "sys_config:"

// builtinYes 内置参数标记，对应 SystemConstants.YES。
const builtinYes = "Y"

// Service 参数配置业务层。cache 可为 nil（降级查库）。
type Service struct {
	repo  *Repo
	auth  authctx.Context
	cache *cachex.Client
}

// NewService 构造业务层。
func NewService(db *gorm.DB, ac authctx.Context, cache *cachex.Client) *Service {
	return &Service{repo: NewRepo(db), auth: ac, cache: cache}
}

// errDuplicateKey 哨兵：键名已存在（handler 据此拼带 config_name 的文案）。
var errDuplicateKey = errs.New("参数键名已存在")

// ---- 查询 ----

// buildConditions 对应 _build_conditions。
//
// ⚠️ config_type 是**等值**，config_name / config_key 是 like。
func buildConditions(q Query) []crudx.Cond {
	conds := make([]crudx.Cond, 0, 4)
	if crudx.IsNotBlank(q.ConfigName) {
		conds = append(conds, crudx.Like("config_name", q.ConfigName))
	}
	if crudx.IsNotBlank(q.ConfigType) {
		conds = append(conds, crudx.Eq("config_type", q.ConfigType))
	}
	if crudx.IsNotBlank(q.ConfigKey) {
		conds = append(conds, crudx.Like("config_key", q.ConfigKey))
	}
	if q.BeginTime != "" && q.EndTime != "" {
		conds = append(conds, crudx.Between("create_time", q.BeginTime, q.EndTime))
	}
	return conds
}

// Page 分页查询，对应 select_page_config_list。
func (s *Service) Page(ctx context.Context, q Query) ([]Out, int64, error) {
	rows, total, err := s.repo.Page(ctx, q.Page, buildConditions(q))
	if err != nil {
		return nil, 0, err
	}
	return outs(rows), total, nil
}

// ListAll 不分页查询（导出用），对应 select_config_list。
func (s *Service) ListAll(ctx context.Context, q Query) ([]Out, error) {
	rows, err := s.repo.ListAll(ctx, buildConditions(q)...)
	if err != nil {
		return nil, err
	}
	return outs(rows), nil
}

// GetByID 按主键查询，对应 select_config_by_id。
func (s *Service) GetByID(ctx context.Context, id int64) (*Out, error) {
	row, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	out := NewOut(row)
	return &out, nil
}

// GetByKey 按键名查参数值（缓存优先），对应 select_config_by_key。
//
// 未找到返回空串（不是 null）——Python 端 `value = instance.config_value ... or ""`。
func (s *Service) GetByKey(ctx context.Context, key string) (string, error) {
	cacheKey := cacheConfigKey + key
	if s.cache != nil {
		if raw, ok := s.cache.Get(ctx, cacheKey); ok {
			// 兼容带引号的 JSON 字符串值（历史写入形态），能解包就解包
			if len(raw) >= 2 && strings.HasPrefix(raw, `"`) && strings.HasSuffix(raw, `"`) {
				var unwrapped string
				if err := json.Unmarshal([]byte(raw), &unwrapped); err == nil {
					return unwrapped, nil
				}
			}
			return raw, nil
		}
	}
	row, err := s.repo.GetByKey(ctx, key)
	if err != nil {
		return "", errs.Wrap(err, errs.MsgDBFailed)
	}
	value := ""
	if row != nil && row.ConfigValue != nil {
		value = *row.ConfigValue
	}
	if s.cache != nil {
		s.cache.Set(ctx, cacheKey, value, 0)
	}
	return value, nil
}

// ---- 校验 ----

// CheckKeyUnique 键名唯一性，对应 check_config_key_unique。
func (s *Service) CheckKeyUnique(ctx context.Context, key *string, excludeID *int64) error {
	if key == nil {
		return errs.New("参数键名不能为空")
	}
	exists, err := s.repo.ExistsByKey(ctx, *key, excludeID)
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	if exists {
		return errDuplicateKey
	}
	return nil
}

// ---- 写操作 ----

// Insert 新增参数配置，对应 insert_config。
//
// ⚠️ 缺省字段走列默认值（config_name/key/value=""、config_type="N"、remark=NULL），
// 与 dict 数据表同语义（model_dump 后过滤 None 再构造）。
func (s *Service) Insert(ctx context.Context, in CreateIn) error {
	if in.ConfigKey == nil {
		return errs.New("参数键名不能为空")
	}
	if in.ConfigValue == nil {
		return errs.New("参数键值不能为空")
	}
	row := SysConfig{}
	if in.ConfigName != nil {
		row.ConfigName = in.ConfigName
	} else {
		row.ConfigName = strPtr("")
	}
	row.ConfigKey = in.ConfigKey
	row.ConfigValue = in.ConfigValue
	if in.ConfigType != nil {
		row.ConfigType = in.ConfigType
	} else {
		row.ConfigType = strPtr("N")
	}
	row.Remark = in.Remark

	crudx.FillCreate(&row.BaseModel, s.auth, nowFunc())
	if err := s.repo.Create(ctx, &row); err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	if s.cache != nil {
		s.cache.Set(ctx, cacheConfigKey+*in.ConfigKey, *in.ConfigValue, 0)
	}
	return nil
}

// updateParam 两种修改入参的统一形态。
type updateParam struct {
	ID          *int64
	ConfigName  *string
	ConfigKey   *string
	ConfigValue *string
	ConfigType  *string
	Remark      *string
}

// Update 修改参数配置（有 id 按主键、无 id 按键名批量），对应 update_config。
func (s *Service) Update(ctx context.Context, p updateParam) error {
	if p.ConfigKey == nil {
		return errs.New("参数键名不能为空")
	}
	var affected int64
	if p.ID != nil {
		row, err := s.repo.Get(ctx, *p.ID)
		if err != nil {
			return errs.Wrap(err, errs.MsgDBFailed)
		}
		if row == nil {
			return errs.New("操作失败")
		}
		// 改键名时先删旧键缓存
		if s.cache != nil && deref(row.ConfigKey) != *p.ConfigKey {
			s.cache.Delete(ctx, cacheConfigKey+deref(row.ConfigKey))
		}
		if p.ConfigName != nil {
			row.ConfigName = p.ConfigName
		}
		row.ConfigKey = p.ConfigKey
		if p.ConfigValue != nil {
			row.ConfigValue = p.ConfigValue
		}
		if p.ConfigType != nil {
			row.ConfigType = p.ConfigType
		}
		if p.Remark != nil {
			row.Remark = p.Remark
		}
		crudx.FillUpdate(&row.BaseModel, s.auth, nowFunc())
		if err := s.repo.Update(ctx, row); err != nil {
			return errs.Wrap(err, errs.MsgDBFailed)
		}
		affected = 1
	} else {
		if s.cache != nil {
			s.cache.Delete(ctx, cacheConfigKey+*p.ConfigKey)
		}
		values := map[string]any{}
		if p.ConfigName != nil {
			values["config_name"] = *p.ConfigName
		}
		if p.ConfigKey != nil {
			values["config_key"] = *p.ConfigKey
		}
		if p.ConfigValue != nil {
			values["config_value"] = *p.ConfigValue
		}
		if p.ConfigType != nil {
			values["config_type"] = *p.ConfigType
		}
		if p.Remark != nil {
			values["remark"] = *p.Remark
		}
		if len(values) > 0 {
			uid := s.auth.User.ID
			if uid == 0 {
				uid = crudx.DefaultUserID
			}
			n, err := s.repo.UpdateByKeyBulk(ctx, *p.ConfigKey, values, uid, nowFunc())
			if err != nil {
				return errs.Wrap(err, errs.MsgDBFailed)
			}
			affected = n
		}
	}
	if affected > 0 {
		// Python 端 set(key, req.config_value)；值为 None 时 redis-py 会抛错被静默吞掉
		// → 等价于不写缓存，Go 侧直接跳过
		if s.cache != nil && p.ConfigValue != nil {
			s.cache.Set(ctx, cacheConfigKey+*p.ConfigKey, *p.ConfigValue, 0)
		}
		return nil
	}
	return errs.New("操作失败")
}

// Delete 批量删除参数配置，对应 delete_config_by_ids。
//
// ⚠️ 内置参数（config_type='Y'）不允许删除；缓存删除在循环内逐条进行，
// 抛错前已删的键**不会回滚**（Python 端同样如此），照抄。
func (s *Service) Delete(ctx context.Context, ids []int64) error {
	rows, err := s.repo.ListAll(ctx, crudx.In("id", ids))
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	for i := range rows {
		if deref(rows[i].ConfigType) == builtinYes {
			return errs.Newf("内置参数【%s】不能删除", deref(rows[i].ConfigKey))
		}
		if s.cache != nil {
			s.cache.Delete(ctx, cacheConfigKey+deref(rows[i].ConfigKey))
		}
	}
	if _, err := s.repo.DeleteBatch(ctx, ids); err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	return nil
}

// ResetCache 重置参数缓存，对应 reset_config_cache。
func (s *Service) ResetCache(ctx context.Context) {
	if s.cache == nil {
		return
	}
	s.cache.DeleteByPattern(ctx, cacheConfigKey+"*")
}

// ---- 内部工具 ----

func outs(rows []SysConfig) []Out {
	out := make([]Out, 0, len(rows))
	for i := range rows {
		out = append(out, NewOut(&rows[i]))
	}
	return out
}
