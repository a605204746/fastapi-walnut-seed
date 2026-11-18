/**
 * 对应 Vue 侧 `views/system/dict/data.vue` —— **占位页，无实际业务意义**。
 *
 * 存在理由：后端 `sys_menu.component` 里有一行 `system/dict/data`
 * （菜单「字典数据」，路径 `dict-data/index/:dictId`，`hidden: true`）。
 *
 * ⚠️ 注意解析优先级：`generateRoutesByBackend` 把 `system/dict/data` 拼成
 * `/system/dict/data.tsx` 后**精确匹配** pageMap，所以命中的一定是本文件，
 * 而**不是** `system/dict/data/index.tsx`。Vue 侧 `.vue` 后缀下同理
 * （`data.vue` 优先于 `data/index.vue`）——两端行为一致，故这里保持占位。
 * 真正的字典数据管理在 `system/dict/index.tsx` 内以弹窗形式完成，
 * `system/dict/data/` 目录放的是那两个弹窗组件。
 *
 * ⚠️ 文件必须有 `export default`（见 `system/user/authRole.tsx` 的说明）。
 */
export function DictDataPlaceholder() {
  return (
    <div>
      ele版本会使用这个文件 只是为了不报错`未找到对应组件`才新建的这个文件
      无实际意义
    </div>
  );
}

export default DictDataPlaceholder;
