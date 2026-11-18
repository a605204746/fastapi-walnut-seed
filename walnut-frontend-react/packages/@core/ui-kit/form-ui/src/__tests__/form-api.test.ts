import type { FormSchema } from '../types';

import { afterEach, describe, expect, it, vi } from 'vitest';

import { FormApi } from '../form-api';

/**
 * `FormApi.updateSchema` 的用例。
 *
 * 这个方法是 8 个抽屉页的硬前置：它们都在 `onOpenChange` 里先拉异步数据
 * （部门树 / 字典 / 岗位下拉），再用 `updateSchema` 把 `componentProps` 注入进去
 * （见 Vue 侧 `views/system/post/post-drawer.vue` 的 `setupDeptSelect`）。
 *
 * 语义需与 Vue 侧 `form-ui/src/form-api.ts:384` 逐条对齐：
 * 每项必须带 fieldName、只更新已存在字段、深合并 + 数组整体覆盖、多次调用累积。
 */

function buildSchema(): FormSchema[] {
  return [
    { component: 'Input', fieldName: 'name', label: '名称' },
    {
      component: 'Select',
      componentProps: { options: [] as any[] },
      fieldName: 'deptId',
      label: '部门',
    },
    { component: 'Input', fieldName: 'remark', rules: 'required' },
  ];
}

function field(api: FormApi, name: string): FormSchema {
  const found = (api.latestOptions.schema ?? []).find(
    (item) => item.fieldName === name,
  );
  if (!found) throw new Error(`字段不存在: ${name}`);
  return found;
}

function revisionOf(api: FormApi): number {
  return (api.getStore().state as { revision: number }).revision;
}

describe('FormApi.updateSchema', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('合并 componentProps，保留原有字段属性', async () => {
    const api = new FormApi({ schema: buildSchema() });
    await api.updateSchema([
      {
        componentProps: { treeData: [{ id: 1, label: '研发部' }] },
        fieldName: 'deptId',
      },
    ]);

    const dept = field(api, 'deptId');
    expect(dept.label).toBe('部门');
    expect(dept.component).toBe('Select');
    expect(dept.componentProps).toMatchObject({
      options: [],
      treeData: [{ id: 1, label: '研发部' }],
    });
  });

  it('数组整体覆盖而非合并（mergeWithArrayOverride 语义）', async () => {
    const api = new FormApi({ schema: buildSchema() });
    await api.updateSchema([
      { componentProps: { options: [1, 2, 3] }, fieldName: 'deptId' },
    ]);
    await api.updateSchema([
      { componentProps: { options: [9] }, fieldName: 'deptId' },
    ]);
    expect(field(api, 'deptId').componentProps).toMatchObject({ options: [9] });
  });

  it('多次调用累积生效', async () => {
    const api = new FormApi({ schema: buildSchema() });
    await api.updateSchema([{ fieldName: 'name', label: '名称2' }]);
    await api.updateSchema([{ fieldName: 'name', hideLabel: true }]);

    const name = field(api, 'name');
    expect(name.label).toBe('名称2');
    expect(name.hideLabel).toBe(true);
  });

  it('一次调用可更新多个字段', async () => {
    const api = new FormApi({ schema: buildSchema() });
    await api.updateSchema([
      { fieldName: 'name', label: 'A' },
      { fieldName: 'remark', label: 'B' },
    ]);
    expect(field(api, 'name').label).toBe('A');
    expect(field(api, 'remark').label).toBe('B');
  });

  it('任一项缺 fieldName 则整批不生效并报错（与 Vue 侧一致）', async () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    const api = new FormApi({ schema: buildSchema() });

    await api.updateSchema([
      { fieldName: 'name', label: 'A' },
      { label: 'B' } as Partial<FormSchema>,
    ]);

    expect(errorSpy).toHaveBeenCalledTimes(1);
    // 整批回滚：name 也不该被改
    expect(field(api, 'name').label).toBe('名称');
  });

  it('不会新增 schema 里不存在的字段', async () => {
    const api = new FormApi({ schema: buildSchema() });
    await api.updateSchema([{ component: 'Input', fieldName: 'ghost' }]);

    const names = (api.latestOptions.schema ?? []).map((i) => i.fieldName);
    expect(names).not.toContain('ghost');
    expect(names).toEqual(['name', 'deptId', 'remark']);
  });

  it('bump revision 以触发 FormRenderer 重渲染', async () => {
    const api = new FormApi({ schema: buildSchema() });
    const before = revisionOf(api);
    await api.updateSchema([{ fieldName: 'name', label: 'A' }]);
    expect(revisionOf(api)).toBe(before + 1);
  });

  it('不重置已填的值', async () => {
    const api = new FormApi({ schema: buildSchema() });
    await api.setValues({ name: '张三' });
    await api.updateSchema([{ fieldName: 'name', label: 'A' }]);
    expect(await api.getValues()).toMatchObject({ name: '张三' });
  });

  it('更新后的 rules 立即参与 validate', async () => {
    const api = new FormApi({ schema: buildSchema() });
    // name 原本无 rules，填不填都能过
    expect((await api.validate()).valid).toBe(false); // remark 是 required
    await api.setValues({ remark: 'x' });
    expect((await api.validate()).valid).toBe(true);

    await api.updateSchema([{ fieldName: 'name', rules: 'required' }]);
    const result = await api.validate();
    expect(result.valid).toBe(false);
    expect(result.errors.name).toBeTruthy();
  });

  it('⚠️ React 特有：syncOptions 重跑后覆盖层不丢失', async () => {
    /**
     * `useVbenForm` 每次渲染都会调 `syncOptions(options)`，而业务页通常传的是
     * 内联字面量（`schema: drawerSchema()`），引用每次都变。若覆盖层没被保留，
     * 任何一次重渲染都会把 `updateSchema` 注入的异步选项冲掉 —— 这是 Vue 侧
     * 不存在的问题（Vue 的 schema 就存在 store 里）。
     */
    const api = new FormApi({ schema: buildSchema() });
    await api.updateSchema([
      { componentProps: { treeData: [{ id: 1 }] }, fieldName: 'deptId' },
    ]);

    // 模拟父组件重渲染：传入一个内容相同但引用不同的新 schema
    api.syncOptions({ schema: buildSchema() });

    expect(field(api, 'deptId').componentProps).toMatchObject({
      treeData: [{ id: 1 }],
    });
  });

  it('syncOptions 传入新增字段时补默认值，且不冲掉覆盖层', async () => {
    const api = new FormApi({ schema: buildSchema() });
    await api.updateSchema([{ fieldName: 'name', label: 'A' }]);

    const next = [...buildSchema(), {
      component: 'Input',
      defaultValue: 'x',
      fieldName: 'extra',
    }] as FormSchema[];
    api.syncOptions({ schema: next });

    expect(await api.getValues()).toMatchObject({ extra: 'x' });
    expect(field(api, 'name').label).toBe('A');
  });
});
