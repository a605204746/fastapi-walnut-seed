// 本文件实现字典域的业务逻辑，对应 Python 端 module_system/dict/service.py。
//
// # 缓存语义（逐条镜像，键名与 Python 端 CacheNames 一致）
//
//   - 字典数据：`sys_dict:{dict_type}`，**空集合也缓存**（"[]" 防穿透）；
//     命中且形状合法（dict 列表）直接返回，空列表返回 nil（controller 兜底 []）
//   - 字典类型：`sys_dict_type{dict_type}` —— 仅被 update/delete 清理，
//     当前无端点读它（Python 端 select_dict_type_by_type 也无路由调用），照抄
//   - Redis 不可用（cache == nil）降级为直接查库，与 Python `_get_redis` 返回 None 同语义
//   - refreshCache 的分布式锁在 Python 端「未抢到也直接执行」（清缓存可重入），
//     对响应无可观测影响，故 Go 侧**不实现锁**，只实现清缓存本身

package dict

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/cachex"
	"walnut-seed-go/internal/pkg/crudx"
	"walnut-seed-go/internal/pkg/errs"
)

// nowFunc 提供「当前时间」，抽成变量便于单测钉住审计字段。
var nowFunc = time.Now

// 缓存键前缀，对应 Python 端 CacheNames.SYS_DICT_KEY / SYS_DICT_TYPE。
//
// ⚠️ SYS_DICT_TYPE **没有冒号**（Python 端就是 "sys_dict_type"），照抄。
const (
	cacheDictData = "sys_dict:"
	cacheDictType = "sys_dict_type"
)

// Service 字典业务层。cache 可为 nil（Redis 不可用降级）。
type Service struct {
	typeRepo *TypeRepo
	dataRepo *DataRepo
	auth     authctx.Context
	cache    *cachex.Client
}

// NewService 构造业务层。
func NewService(db *gorm.DB, ac authctx.Context, cache *cachex.Client) *Service {
	return &Service{typeRepo: NewTypeRepo(db), dataRepo: NewDataRepo(db), auth: ac, cache: cache}
}

// ---- 字典类型查询 ----

// buildTypeConditions 对应 DictTypeService._build_conditions。
func buildTypeConditions(q TypeQuery) []crudx.Cond {
	conds := make([]crudx.Cond, 0, 3)
	if crudx.IsNotBlank(q.DictName) {
		conds = append(conds, crudx.Like("dict_name", q.DictName))
	}
	if crudx.IsNotBlank(q.DictType) {
		conds = append(conds, crudx.Like("dict_type", q.DictType))
	}
	if q.BeginTime != "" && q.EndTime != "" {
		conds = append(conds, crudx.Between("create_time", q.BeginTime, q.EndTime))
	}
	return conds
}

// TypePage 分页查询字典类型，对应 select_page_dict_type_list。
func (s *Service) TypePage(ctx context.Context, q TypeQuery) ([]TypeOut, int64, error) {
	rows, total, err := s.typeRepo.Page(ctx, q.Page, buildTypeConditions(q))
	if err != nil {
		return nil, 0, err
	}
	return typeOuts(rows), total, nil
}

// TypeListAll 不分页查询（导出用），对应 select_dict_type_list。
func (s *Service) TypeListAll(ctx context.Context, q TypeQuery) ([]TypeOut, error) {
	rows, err := s.typeRepo.ListAll(ctx, buildTypeConditions(q)...)
	if err != nil {
		return nil, err
	}
	return typeOuts(rows), nil
}

// TypeOptionSelect 全部字典类型（optionselect 用），对应 select_dict_type_all。
func (s *Service) TypeOptionSelect(ctx context.Context) ([]TypeOut, error) {
	rows, err := s.typeRepo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	return typeOuts(rows), nil
}

// TypeGet 按 ID 查询，对应 select_dict_type_by_id（不存在返回 nil）。
func (s *Service) TypeGet(ctx context.Context, id int64) (*TypeOut, error) {
	row, err := s.typeRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	out := NewTypeOut(row)
	return &out, nil
}

// ---- 字典数据查询 ----

// buildDataConditions 对应 DictDataService._build_conditions。
//
// ⚠️ dict_type 是**等值**匹配（不是 like），与 type 列表的 like 不同。
func buildDataConditions(q DataQuery) []crudx.Cond {
	conds := make([]crudx.Cond, 0, 3)
	if q.DictSort != nil {
		conds = append(conds, crudx.Eq("dict_sort", *q.DictSort))
	}
	if crudx.IsNotBlank(q.DictLabel) {
		conds = append(conds, crudx.Like("dict_label", q.DictLabel))
	}
	if crudx.IsNotBlank(q.DictType) {
		conds = append(conds, crudx.Eq("dict_type", q.DictType))
	}
	return conds
}

// DataPage 分页查询字典数据，对应 select_page_dict_data_list。
func (s *Service) DataPage(ctx context.Context, q DataQuery) ([]DataOut, int64, error) {
	rows, total, err := s.dataRepo.Page(ctx, q.Page, buildDataConditions(q))
	if err != nil {
		return nil, 0, err
	}
	return dataOuts(rows), total, nil
}

// DataListAll 不分页查询（导出用），对应 select_dict_data_list。
func (s *Service) DataListAll(ctx context.Context, q DataQuery) ([]DataOut, error) {
	rows, err := s.dataRepo.ListAll(ctx, buildDataConditions(q)...)
	if err != nil {
		return nil, err
	}
	return dataOuts(rows), nil
}

// DataGet 按 ID 查询，对应 select_dict_data_by_id。
func (s *Service) DataGet(ctx context.Context, id int64) (*DataOut, error) {
	row, err := s.dataRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	out := NewDataOut(row)
	return &out, nil
}

// DataByType 按类型查字典数据（缓存优先），对应 select_dict_data_by_type。
//
// 返回 nil 表示「无数据」，由 handler 兜底成 []（Python 端 `data or []`）。
func (s *Service) DataByType(ctx context.Context, dictType string) ([]DataOut, error) {
	key := cacheDictData + dictType
	if s.cache != nil {
		if raw, ok := s.cache.Get(ctx, key); ok {
			var cached []DataOut
			if err := json.Unmarshal([]byte(raw), &cached); err == nil {
				// 仅接受本服务写入的形状（对象列表）；异构缓存视为未命中并重写
				if len(cached) > 0 {
					return cached, nil
				}
				// 空集合缓存命中 → Python `cached or None` → controller 兜底 []
				return nil, nil
			}
		}
	}
	rows, err := s.dataRepo.ListByType(ctx, dictType)
	if err != nil {
		return nil, err
	}
	payload := dataOuts(rows)
	if s.cache != nil {
		s.setCache(ctx, key, payload)
	}
	if len(payload) == 0 {
		return nil, nil
	}
	return payload, nil
}

// setCache 把出参切片写入缓存（JSON 键序 = struct 声明序 = Pydantic 声明序）。
func (s *Service) setCache(ctx context.Context, key string, payload any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	s.cache.Set(ctx, key, string(raw), 0)
}

// ---- 校验 ----

// CheckTypeUnique 字典类型唯一性，对应 check_dict_type_unique。
//
// ⚠️ dict_type 为 None 时抛 500「字典类型不能为空」——缺省字段不跑 schema 校验器，
// 会走到这里（不是 400），照抄。
func (s *Service) CheckTypeUnique(ctx context.Context, dictType *string, excludeID *int64) error {
	if dictType == nil {
		return errs.New("字典类型不能为空")
	}
	exists, err := s.typeRepo.ExistsByType(ctx, *dictType, excludeID)
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	if exists {
		return errDuplicateType
	}
	return nil
}

// errDuplicateType 哨兵：handler 据此输出带 dict_name 的文案。
var errDuplicateType = errs.New("字典类型已存在")

// errDuplicateValue 哨兵：字典键值已存在。
var errDuplicateValue = errs.New("字典键值已存在")

// CheckDataUnique 字典键值唯一性，对应 check_dict_data_unique。
func (s *Service) CheckDataUnique(ctx context.Context, dictType, dictValue *string, excludeID *int64) error {
	if dictType == nil {
		return errs.New("字典类型不能为空")
	}
	if dictValue == nil {
		return errs.New("字典键值不能为空")
	}
	exists, err := s.dataRepo.ExistsByTypeAndValue(ctx, *dictType, *dictValue, excludeID)
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	if exists {
		return errDuplicateValue
	}
	return nil
}

// ---- 写操作 ----

// InsertType 新增字典类型，对应 insert_dict_type。
//
// ⚠️ 字段原样传入构造器：缺省（None）插 **NULL**，不吃列默认值
// （与数据新增的「过滤 None 走默认值」语义相反，照抄）。
func (s *Service) InsertType(ctx context.Context, in TypeCreateIn) error {
	if in.DictType == nil {
		return errs.New("字典类型不能为空")
	}
	row := SysDictType{DictName: in.DictName, DictType: in.DictType, Remark: in.Remark}
	crudx.FillCreate(&row.BaseModel, s.auth, nowFunc())
	if err := s.typeRepo.Create(ctx, &row); err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	// 新类型下无数据：缓存空集合防穿透
	if s.cache != nil {
		s.cache.Set(ctx, cacheDictData+*in.DictType, "[]", 0)
	}
	return nil
}

// UpdateType 修改字典类型，对应 update_dict_type。
func (s *Service) UpdateType(ctx context.Context, in TypeUpdateIn) error {
	if in.DictType == nil {
		return errs.New("字典类型不能为空")
	}
	old, err := s.typeRepo.Get(ctx, *in.ID)
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	if old == nil {
		return errs.New("操作失败")
	}
	oldType := deref(old.DictType)

	// 同步更新字典数据表的字典类型（不填审计字段）
	if _, err := s.dataRepo.RenameType(ctx, oldType, *in.DictType); err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}

	// 对应 model_dump(exclude_unset=True) 去 id 后仅对非 None 字段 setattr
	if in.DictName != nil {
		old.DictName = in.DictName
	}
	if in.DictType != nil {
		old.DictType = in.DictType
	}
	if in.Remark != nil {
		old.Remark = in.Remark
	}
	crudx.FillUpdate(&old.BaseModel, s.auth, nowFunc())
	if err := s.typeRepo.Update(ctx, old); err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}

	if s.cache != nil {
		s.cache.Delete(ctx,
			cacheDictData+oldType,
			cacheDictType+oldType,
			cacheDictData+*in.DictType,
			cacheDictType+*in.DictType,
		)
	}
	return nil
}

// DeleteTypes 批量删除字典类型，对应 delete_dict_type_by_ids。
//
// ⚠️ 占用检查在删除**之前**全量做完：任一类型下有数据就整体抛错、一条不删。
func (s *Service) DeleteTypes(ctx context.Context, ids []int64) error {
	rows, err := s.typeRepo.ListAll(ctx, crudx.In("id", ids))
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	for i := range rows {
		occupied, err := s.dataRepo.ExistsByType(ctx, deref(rows[i].DictType))
		if err != nil {
			return errs.Wrap(err, errs.MsgDBFailed)
		}
		if occupied {
			return errs.Newf("%s已分配,不能删除", deref(rows[i].DictName))
		}
	}
	if _, err := s.typeRepo.DeleteBatch(ctx, ids); err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	if s.cache != nil {
		for i := range rows {
			t := deref(rows[i].DictType)
			s.cache.Delete(ctx, cacheDictData+t, cacheDictType+t)
		}
	}
	return nil
}

// ResetCache 重置字典缓存，对应 reset_dict_cache。
func (s *Service) ResetCache(ctx context.Context) {
	if s.cache == nil {
		return
	}
	s.cache.DeleteByPattern(ctx, cacheDictData+"*")
	s.cache.DeleteByPattern(ctx, cacheDictType+"*")
}

// InsertData 新增字典数据，对应 insert_dict_data。
//
// ⚠️ 缺省/显式 null 字段**不写入**（过滤 None），由列默认值兜底
// （dict_sort=0、dict_label=""、dict_value=""、dict_type=""、is_default="N"）。
func (s *Service) InsertData(ctx context.Context, in DataCreateIn) error {
	if in.DictType == nil {
		return errs.New("字典类型不能为空")
	}
	row := SysDictData{}
	if in.DictSort != nil {
		row.DictSort = in.DictSort
	} else {
		// 列默认值 0（Python 端 DictDataModel(**data) 不含该键时由客户端默认值补）
		row.DictSort = intPtrOf(0)
	}
	if in.DictLabel != nil {
		row.DictLabel = in.DictLabel
	} else {
		row.DictLabel = strPtr("")
	}
	if in.DictValue != nil {
		row.DictValue = in.DictValue
	} else {
		row.DictValue = strPtr("")
	}
	row.DictType = in.DictType
	if in.CssClass != nil {
		row.CssClass = in.CssClass
	}
	if in.ListClass != nil {
		row.ListClass = in.ListClass
	}
	if in.IsDefault != nil {
		row.IsDefault = in.IsDefault
	} else {
		// 列默认值 "N"（css_class/list_class/remark 的默认值是 None → 保持 NULL）
		row.IsDefault = strPtr("N")
	}
	if in.Remark != nil {
		row.Remark = in.Remark
	}
	crudx.FillCreate(&row.BaseModel, s.auth, nowFunc())
	if err := s.dataRepo.Create(ctx, &row); err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	return s.refreshDataCache(ctx, *in.DictType)
}

// UpdateData 修改字典数据，对应 update_dict_data。
//
// ⚠️ 只刷新 **req.dict_type** 的缓存：Python 端改类型后旧类型缓存不刷新，照抄。
func (s *Service) UpdateData(ctx context.Context, in DataUpdateIn) error {
	if in.DictType == nil {
		return errs.New("字典类型不能为空")
	}
	row, err := s.dataRepo.Get(ctx, *in.ID)
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	if row == nil {
		return errs.New("操作失败")
	}
	if in.DictSort != nil {
		row.DictSort = in.DictSort
	}
	if in.DictLabel != nil {
		row.DictLabel = in.DictLabel
	}
	if in.DictValue != nil {
		row.DictValue = in.DictValue
	}
	if in.DictType != nil {
		row.DictType = in.DictType
	}
	if in.CssClass != nil {
		row.CssClass = in.CssClass
	}
	if in.ListClass != nil {
		row.ListClass = in.ListClass
	}
	if in.IsDefault != nil {
		row.IsDefault = in.IsDefault
	}
	if in.Remark != nil {
		row.Remark = in.Remark
	}
	crudx.FillUpdate(&row.BaseModel, s.auth, nowFunc())
	if err := s.dataRepo.Update(ctx, row); err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	return s.refreshDataCache(ctx, *in.DictType)
}

// DeleteData 批量删除字典数据，对应 delete_dict_data_by_ids。
func (s *Service) DeleteData(ctx context.Context, ids []int64) error {
	rows, err := s.dataRepo.ListAll(ctx, crudx.In("id", ids))
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	if _, err := s.dataRepo.DeleteBatch(ctx, ids); err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	if s.cache != nil {
		for i := range rows {
			s.cache.Delete(ctx, cacheDictData+deref(rows[i].DictType))
		}
	}
	return nil
}

// refreshDataCache 重新加载指定类型的全量数据并写缓存，对应 _refresh_dict_data_cache。
func (s *Service) refreshDataCache(ctx context.Context, dictType string) error {
	if s.cache == nil {
		return nil
	}
	rows, err := s.dataRepo.ListByType(ctx, dictType)
	if err != nil {
		return errs.Wrap(err, errs.MsgDBFailed)
	}
	s.setCache(ctx, cacheDictData+dictType, dataOuts(rows))
	return nil
}

// ---- 内部工具 ----

func typeOuts(rows []SysDictType) []TypeOut {
	outs := make([]TypeOut, 0, len(rows))
	for i := range rows {
		outs = append(outs, NewTypeOut(&rows[i]))
	}
	return outs
}

func dataOuts(rows []SysDictData) []DataOut {
	outs := make([]DataOut, 0, len(rows))
	for i := range rows {
		outs = append(outs, NewDataOut(&rows[i]))
	}
	return outs
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func intPtrOf(v int) *int { return &v }
