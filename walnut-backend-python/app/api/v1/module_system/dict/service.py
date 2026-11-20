"""字典类型与字典数据的业务层。

缓存语义：
- 字典数据按 ``sys_dict:{dict_type}`` 缓存（空集合也缓存，防止缓存穿透）；
- 字典类型按 ``sys_dict_type{dict_type}`` 缓存；
- Redis 不可用时降级为直接查库（``redis`` 参数传 None 即可）。
"""

import json

from redis.asyncio.client import Redis
from sqlalchemy import update
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.v1.module_system.dict.crud import DictDataCrud, DictTypeCrud
from app.api.v1.module_system.dict.model import DictDataModel, DictTypeModel
from app.api.v1.module_system.dict.schema import (
    DictDataCreateSchema,
    DictDataOutSchema,
    DictDataQueryParam,
    DictDataUpdateSchema,
    DictTypeCreateSchema,
    DictTypeOutSchema,
    DictTypeQueryParam,
    DictTypeUpdateSchema,
)
from app.common.enums import CacheNames
from app.core.base_schema import AuthSchema
from app.core.exceptions import ServiceException
from app.core.redis_crud import RedisUtils
from app.utils.string_util import is_not_blank


class DictTypeService:
    """字典类型业务层。"""

    def __init__(self, auth: AuthSchema, db: AsyncSession) -> None:
        self.auth = auth
        self.db = db
        self.dict_type_crud = DictTypeCrud(DictTypeModel, auth, db)
        self.dict_data_crud = DictDataCrud(DictDataModel, auth, db)

    # ---------------- 查询 ----------------
    def _build_conditions(self, param: DictTypeQueryParam) -> list:
        conditions = []
        if is_not_blank(param.dict_name):
            conditions.append(DictTypeModel.dict_name.like(f"%{param.dict_name}%"))
        if is_not_blank(param.dict_type):
            conditions.append(DictTypeModel.dict_type.like(f"%{param.dict_type}%"))
        if param.begin_time is not None and param.end_time is not None:
            conditions.append(DictTypeModel.create_time.between(param.begin_time, param.end_time))
        return conditions

    async def select_page_dict_type_list(self, param: DictTypeQueryParam) -> dict:
        """分页查询字典类型列表。"""
        return await self.dict_type_crud.page(param, *self._build_conditions(param))

    async def select_dict_type_list(self, param: DictTypeQueryParam) -> list[DictTypeModel]:
        """查询字典类型列表（导出用）。"""
        return await self.dict_type_crud.list_all(*self._build_conditions(param))

    async def select_dict_type_all(self) -> list[DictTypeModel]:
        """查询所有字典类型（optionselect 用）。"""
        return await self.dict_type_crud.list_all()

    async def select_dict_type_by_id(self, dict_id: int) -> DictTypeModel | None:
        """根据ID查询字典类型。"""
        return await self.dict_type_crud.get(dict_id)

    async def select_dict_type_by_type(self, dict_type: str, redis: Redis | None) -> dict | None:
        """根据字典类型查询字典类型信息（缓存优先）。"""
        cache_key = CacheNames.SYS_DICT_TYPE + dict_type
        if redis is not None:
            raw = await RedisUtils(redis).get(cache_key)
            if raw:
                try:
                    cached = json.loads(raw)
                except (TypeError, json.JSONDecodeError):
                    cached = None
                if isinstance(cached, dict):
                    return cached
        instance = await self.dict_type_crud.get_by(dict_type=dict_type)
        if instance is None:
            return None
        payload = DictTypeOutSchema.model_validate(instance).model_dump(by_alias=True, mode="json")
        if redis is not None:
            await RedisUtils(redis).set(cache_key, json.dumps(payload, ensure_ascii=False))
        return payload

    async def select_dict_data_by_type(self, dict_type: str, redis: Redis | None) -> list[dict] | None:
        """根据字典类型查询字典数据（缓存优先，空集合也缓存防穿透）。"""
        cache_key = CacheNames.SYS_DICT_KEY + dict_type
        if redis is not None:
            raw = await RedisUtils(redis).get(cache_key)
            if raw:
                try:
                    cached = json.loads(raw)
                except (TypeError, json.JSONDecodeError):
                    cached = None
                # 仅接受本服务写入的形状（dict 列表）；异构缓存格式视为未命中并重写
                if isinstance(cached, list) and all(isinstance(i, dict) for i in cached):
                    return cached or None
        rows = await self.dict_data_crud.list_by_type(dict_type)
        payload = [DictDataOutSchema.model_validate(row).model_dump(by_alias=True, mode="json") for row in rows]
        if redis is not None:
            await RedisUtils(redis).set(cache_key, json.dumps(payload, ensure_ascii=False))
        return payload or None

    # ---------------- 写操作 ----------------
    async def insert_dict_type(self, req: DictTypeCreateSchema, redis: Redis | None) -> None:
        """新增字典类型（新增类型下无数据，缓存空集合防穿透）。"""
        if req.dict_type is None:
            raise ServiceException("字典类型不能为空")
        instance = DictTypeModel(dict_name=req.dict_name, dict_type=req.dict_type, remark=req.remark)
        await self.dict_type_crud.create(instance)
        if redis is not None:
            await RedisUtils(redis).set(CacheNames.SYS_DICT_KEY + req.dict_type, json.dumps([], ensure_ascii=False))

    async def update_dict_type(self, req: DictTypeUpdateSchema, redis: Redis | None) -> None:
        """修改字典类型（同步更新字典数据表中的字典类型并清理缓存）。"""
        if req.dict_type is None:
            raise ServiceException("字典类型不能为空")
        old = await self.dict_type_crud.get(req.id)
        if old is None:
            raise ServiceException("操作失败")
        old_type = old.dict_type
        assert old_type is not None
        # 同步更新字典数据表的字典类型
        await self.db.execute(update(DictDataModel).where(DictDataModel.dict_type == old_type).values(dict_type=req.dict_type))
        data = req.model_dump(exclude_unset=True)
        data.pop("id", None)
        for field, value in data.items():
            if value is not None:
                setattr(old, field, value)
        await self.dict_type_crud.update(old)
        # 事务内只做删除，提交后下次读取自动重建缓存
        if redis is not None:
            await RedisUtils(redis).delete(
                CacheNames.SYS_DICT_KEY + old_type,
                CacheNames.SYS_DICT_TYPE + old_type,
                CacheNames.SYS_DICT_KEY + req.dict_type,
                CacheNames.SYS_DICT_TYPE + req.dict_type,
            )

    async def delete_dict_type_by_ids(self, dict_ids: list[int], redis: Redis | None) -> None:
        """批量删除字典类型（已分配字典数据的类型不允许删除）。"""
        type_list = await self.dict_type_crud.list_all(DictTypeModel.id.in_(dict_ids))
        for item in type_list:
            assert item.dict_type is not None
            if await self.dict_data_crud.exists_by_type(item.dict_type):
                raise ServiceException(f"{item.dict_name or ''}已分配,不能删除")
        await self.dict_type_crud.delete_batch(dict_ids)
        if redis is not None:
            ru = RedisUtils(redis)
            for item in type_list:
                assert item.dict_type is not None
                await ru.delete(CacheNames.SYS_DICT_KEY + item.dict_type, CacheNames.SYS_DICT_TYPE + item.dict_type)

    async def reset_dict_cache(self, redis: Redis | None) -> None:
        """重置字典缓存。"""
        if redis is None:
            return
        await RedisUtils(redis).delete_by_pattern(CacheNames.SYS_DICT_KEY + "*")
        await RedisUtils(redis).delete_by_pattern(CacheNames.SYS_DICT_TYPE + "*")

    async def check_dict_type_unique(self, req: DictTypeCreateSchema | DictTypeUpdateSchema) -> bool:
        """校验字典类型是否唯一。"""
        if req.dict_type is None:
            raise ServiceException("字典类型不能为空")
        exclude_id = getattr(req, "id", None)
        return not await self.dict_type_crud.exists_by_dict_type(req.dict_type, exclude_id=exclude_id)


class DictDataService:
    """字典数据业务层。"""

    def __init__(self, auth: AuthSchema, db: AsyncSession) -> None:
        self.auth = auth
        self.db = db
        self.dict_data_crud = DictDataCrud(DictDataModel, auth, db)

    # ---------------- 查询 ----------------
    def _build_conditions(self, param: DictDataQueryParam) -> list:
        conditions = []
        if param.dict_sort is not None:
            conditions.append(DictDataModel.dict_sort == param.dict_sort)
        if is_not_blank(param.dict_label):
            conditions.append(DictDataModel.dict_label.like(f"%{param.dict_label}%"))
        if is_not_blank(param.dict_type):
            conditions.append(DictDataModel.dict_type == param.dict_type)
        return conditions

    async def select_page_dict_data_list(self, param: DictDataQueryParam) -> dict:
        """分页查询字典数据列表。"""
        return await self.dict_data_crud.page(param, *self._build_conditions(param))

    async def select_dict_data_list(self, param: DictDataQueryParam) -> list[DictDataModel]:
        """查询字典数据列表（导出用）。"""
        return await self.dict_data_crud.list_all(*self._build_conditions(param))

    async def select_dict_data_by_id(self, dict_code: int) -> DictDataModel | None:
        """根据ID查询字典数据。"""
        return await self.dict_data_crud.get(dict_code)

    async def select_dict_label(self, dict_type: str, dict_value: str) -> str | None:
        """根据字典类型和键值查询标签。"""
        instance = await self.dict_data_crud.get_by(dict_type=dict_type, dict_value=dict_value)
        return instance.dict_label if instance else None

    # ---------------- 写操作 ----------------
    async def _refresh_dict_data_cache(self, redis: Redis | None, dict_type: str) -> None:
        """重新加载指定字典类型的全量字典数据并写入缓存。"""
        if redis is None:
            return
        rows = await self.dict_data_crud.list_by_type(dict_type)
        payload = [DictDataOutSchema.model_validate(row).model_dump(by_alias=True, mode="json") for row in rows]
        await RedisUtils(redis).set(CacheNames.SYS_DICT_KEY + dict_type, json.dumps(payload, ensure_ascii=False))

    async def insert_dict_data(self, req: DictDataCreateSchema, redis: Redis | None) -> None:
        """新增字典数据（写后刷新缓存）。"""
        if req.dict_type is None:
            raise ServiceException("字典类型不能为空")
        data = {k: v for k, v in req.model_dump(exclude_unset=True).items() if v is not None}
        instance = DictDataModel(**data)
        await self.dict_data_crud.create(instance)
        assert instance.dict_type is not None
        await self._refresh_dict_data_cache(redis, instance.dict_type)

    async def update_dict_data(self, req: DictDataUpdateSchema, redis: Redis | None) -> None:
        """修改字典数据（写后刷新缓存）。"""
        if req.dict_type is None:
            raise ServiceException("字典类型不能为空")
        instance = await self.dict_data_crud.get(req.id)
        if instance is None:
            raise ServiceException("操作失败")
        data = req.model_dump(exclude_unset=True)
        data.pop("id", None)
        for field, value in data.items():
            if value is not None:
                setattr(instance, field, value)
        await self.dict_data_crud.update(instance)
        await self._refresh_dict_data_cache(redis, req.dict_type)

    async def delete_dict_data_by_ids(self, dict_codes: list[int], redis: Redis | None) -> None:
        """批量删除字典数据（删除受影响字典类型的缓存）。"""
        data_list = await self.dict_data_crud.list_all(DictDataModel.id.in_(dict_codes))
        await self.dict_data_crud.delete_batch(dict_codes)
        if redis is not None:
            ru = RedisUtils(redis)
            for item in data_list:
                assert item.dict_type is not None
                await ru.delete(CacheNames.SYS_DICT_KEY + item.dict_type)

    async def check_dict_data_unique(self, req: DictDataCreateSchema | DictDataUpdateSchema) -> bool:
        """校验字典键值是否唯一。"""
        if req.dict_type is None:
            raise ServiceException("字典类型不能为空")
        if req.dict_value is None:
            raise ServiceException("字典键值不能为空")
        exclude_id = getattr(req, "id", None)
        return not await self.dict_data_crud.exists_by_type_and_value(req.dict_type, req.dict_value, exclude_id=exclude_id)
