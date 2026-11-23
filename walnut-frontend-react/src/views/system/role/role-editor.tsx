import { useEffect, useRef, useState } from 'react';
import {
  Button,
  Drawer,
  Form,
  Input,
  InputNumber,
  Modal,
  Radio,
  Select,
  Space,
  Spin,
  Tree,
} from 'antd';
import { DictEnum } from '@/core/constants';
import { roleAdd, roleDataScope, roleDeptTree, roleInfo, roleUpdate } from '@/api/system/role';
import { menuTreeSelect, roleMenuTreeSelect } from '@/api/system/menu';
import type { MenuOption } from '@/api/system/menu/model';
import type { DeptOption, Role } from '@/api/system/role/model';
import { MenuSelectTable } from '@/components/tree';
import { allKeys, withAncestors, type PermissionKey } from '@/components/tree/permission-keys';
import { useDirtyClose } from '@/hooks/use-dirty-close';
import { useDictStore } from '@/store/dict';
import { getDictOptions } from '@/utils/dict';
import { authScopeOptions } from './data';

export function RoleEditor({
  id,
  scope,
  onClose,
  onSaved,
}: {
  id?: number | string;
  scope?: boolean;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [form] = Form.useForm();
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [menus, setMenus] = useState<MenuOption[]>([]);
  const [depts, setDepts] = useState<DeptOption[]>([]);
  const [menuIds, setMenuIds] = useState<PermissionKey[]>([]);
  const [deptIds, setDeptIds] = useState<PermissionKey[]>([]);
  const [associated, setAssociated] = useState(true);
  const [deptAssociated, setDeptAssociated] = useState(true);
  const current = useRef({ menuIds, deptIds, associated, deptAssociated });
  current.current = { menuIds, deptIds, associated, deptAssociated };
  const dirty = useDirtyClose(() => ({ ...form.getFieldsValue(true), ...current.current }));
  useDictStore((s) => s.map);
  const dataScope = Form.useWatch('dataScope', form);
  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const record = id
          ? await roleInfo(id)
          : { roleSort: 1, status: '0', menuCheckStrictly: true, deptCheckStrictly: true };
        const tree =
          scope && id
            ? await roleDeptTree(id)
            : id
              ? await roleMenuTreeSelect(id)
              : { menus: await menuTreeSelect(), checkedKeys: [] };
        if (cancelled) return;
        form.setFieldsValue(record);
        const next = {
          menuIds: 'menus' in tree ? tree.checkedKeys : [],
          deptIds: 'depts' in tree ? tree.checkedKeys : [],
          associated: record.menuCheckStrictly ?? true,
          deptAssociated: record.deptCheckStrictly ?? true,
        };
        if ('menus' in tree) setMenus(tree.menus);
        if ('depts' in tree) setDepts(tree.depts);
        setMenuIds(next.menuIds);
        setDeptIds(next.deptIds);
        setAssociated(next.associated);
        setDeptAssociated(next.deptAssociated);
        current.current = next;
        dirty.mark();
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [id, scope, form]);
  async function save() {
    let values;
    try {
      values = await form.validateFields();
    } catch {
      return;
    }
    setBusy(true);
    try {
      if (scope)
        await roleDataScope({
          ...values,
          id,
          deptCheckStrictly: deptAssociated,
          deptIds:
            values.dataScope === '2'
              ? deptAssociated
                ? withAncestors(depts, deptIds)
                : deptIds
              : [],
        });
      else {
        const payload = {
          ...values,
          id,
          menuCheckStrictly: associated,
          menuIds: associated ? withAncestors(menus, menuIds) : menuIds,
        };
        await (id ? roleUpdate(payload) : roleAdd(payload));
      }
      onSaved();
      onClose();
    } catch {
      /* Keep the draft; request errors are displayed by the request client. */
    } finally {
      setBusy(false);
    }
  }
  const footer = (
    <Space>
      <Button onClick={() => dirty.close(onClose)}>取消</Button>
      <Button type="primary" loading={busy} disabled={loading} onClick={() => void save()}>
        确定
      </Button>
    </Space>
  );
  const body = (
    <Spin spinning={loading}>
      <Form form={form} layout="vertical" className="migration-editor-form">
        <div className={scope ? '' : 'grid grid-cols-1 gap-x-4 sm:grid-cols-2'}>
          <Form.Item name="roleName" label="角色名称" rules={[{ required: true }]}>
            <Input disabled={scope} />
          </Form.Item>
          <Form.Item
            name="roleKey"
            label="权限标识"
            rules={[{ required: true }]}
            tooltip="如: test simpleUser等"
          >
            <Input disabled={scope} />
          </Form.Item>
          {scope ? (
            <Form.Item name="dataScope" label="权限范围" extra="更改后需要用户重新登录才能生效">
              <Select options={authScopeOptions} />
            </Form.Item>
          ) : (
            <>
              <Form.Item name="roleSort" label="角色排序" rules={[{ required: true }]}>
                <InputNumber min={0} className="w-full" />
              </Form.Item>
              <Form.Item name="status" label="状态" rules={[{ required: true }]}>
                <Radio.Group
                  optionType="button"
                  buttonStyle="solid"
                  options={getDictOptions(DictEnum.SYS_NORMAL_DISABLE)}
                />
              </Form.Item>
            </>
          )}
        </div>
        {scope ? (
          dataScope === '2' && (
            <div>
              <Space className="mb-3">
                <Radio.Group
                  value={deptAssociated}
                  options={[
                    { label: '节点关联', value: true },
                    { label: '节点独立', value: false },
                  ]}
                  onChange={(e) => setDeptAssociated(e.target.value)}
                />
                <Button
                  size="small"
                  onClick={() => setDeptIds(deptIds.length ? [] : allKeys(depts))}
                >
                  全选/清空
                </Button>
              </Space>
              <Tree
                checkable
                defaultExpandAll
                fieldNames={{ key: 'id', title: 'label' }}
                treeData={depts}
                checkStrictly={!deptAssociated}
                checkedKeys={deptIds}
                onCheck={(keys) =>
                  setDeptIds(
                    (Array.isArray(keys) ? keys : keys.checked).map((key) =>
                      typeof key === 'bigint' ? String(key) : key,
                    ),
                  )
                }
              />
            </div>
          )
        ) : (
          <>
            <Form.Item label="菜单权限">
              <MenuSelectTable
                menus={menus}
                value={menuIds}
                onChange={setMenuIds}
                association={associated}
                onAssociationChange={setAssociated}
              />
            </Form.Item>
            <Form.Item name="remark" label="备注">
              <Input.TextArea rows={3} />
            </Form.Item>
          </>
        )}
      </Form>
    </Spin>
  );
  return scope ? (
    <Modal open title="分配权限" width={550} footer={footer} onCancel={() => dirty.close(onClose)}>
      {body}
    </Modal>
  ) : (
    <Drawer
      open
      title={id ? '编辑' : '新增'}
      width={800}
      extra={footer}
      onClose={() => dirty.close(onClose)}
      mask={{ closable: !busy }}
    >
      {body}
    </Drawer>
  );
}
