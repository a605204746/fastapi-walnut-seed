import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from 'react';

import type { Role } from '@/api/system/user/model';

import { $t } from '@/core/locales';
import { useWalnutForm, type FormSchema } from '@/core/form-ui';
import { useWalnutDrawer } from '@/core/popup-ui';
import { cloneDeep } from '@/core/shared/utils';
import { addFullName, getPopupContainer } from '@/core/utils';
import { Tag } from 'antd';

import { configInfoByKey } from '@/api/system/config';
import { postOptionSelect } from '@/api/system/post';
import {
  findUserInfo,
  getDeptTree,
  userAdd,
  userUpdate,
} from '@/api/system/user';
import { useDictStore } from '@/store/dict';
import { defaultFormValueGetter, useBeforeCloseDiff } from '@/utils/popup';
import { authScopeOptions } from '@/views/system/role/data';

import { drawerSchema } from './data';

export interface UserDrawerHandle {
  openAdd: () => void;
  openEdit: (id: number | string) => void;
}

export interface UserDrawerProps {
  onReload: () => void;
}

/**
 * 用户新增/编辑抽屉（对应 Vue 侧 views/system/user/user-drawer.vue，250 行）。
 *
 * 与 Vue 版的差异：
 * - `deptId` 的 `componentProps` 是函数形态，Vue 在 `onSelect` 里直接写
 *   `formModel.postIds = []`（vee-validate 的响应式 model）；React 侧表单值在
 *   TanStack Store 里，必须走 `formApi.setFieldValue('postIds', [])`。
 * - `genRoleOptionlabel` 用 JSX 替代 Vue 的 `h()`。
 * - `isUpdate` 同样需要 ref 镜像（见 dept-drawer 的说明）。
 */
export const UserDrawer = forwardRef<UserDrawerHandle, UserDrawerProps>(
  function UserDrawer({ onReload }, ref) {
    // 订阅字典 store：性别 / 状态选项异步到达后重渲染
    useDictStore((state) => state.map);
    const [isUpdate, setIsUpdate] = useState(false);
    const isUpdateRef = useRef(false);
    const defaultPasswordRef = useRef('');

    function setUpdateFlag(value: boolean) {
      isUpdateRef.current = value;
      setIsUpdate(value);
    }

    const [BasicForm, formApi] = useWalnutForm({
      commonConfig: {
        componentProps: { className: 'w-full' },
        formItemClass: 'col-span-2',
        labelWidth: 80,
      },
      schema: drawerSchema() as FormSchema[],
      showDefaultActions: false,
      wrapperClass: 'grid-cols-2',
    });

    /** 生成角色的自定义 label（角色名 + 数据权限范围 Tag） */
    function genRoleOptionLabel(role: Role) {
      const found = authScopeOptions.find(
        (item) => item.value === role.dataScope,
      );
      if (!found) {
        return role.roleName;
      }
      return (
        <div className="flex items-center gap-[6px]">
          <span>{role.roleName}</span>
          <Tag color={found.color}>{found.label}</Tag>
        </div>
      );
    }

    /** 岗位的加载 */
    async function setupPostOptions(deptId: number | string) {
      const postListResp = await postOptionSelect(deptId);
      const options = (postListResp ?? []).map((item) => ({
        label: item.postName,
        value: item.id,
      }));
      const placeholder = options.length > 0 ? '请选择' : '该部门下暂无岗位';
      await formApi.updateSchema([
        {
          componentProps: { options, placeholder },
          fieldName: 'postIds',
        },
      ]);
    }

    /** 初始化部门选择 */
    async function setupDeptSelect() {
      const deptTree = await getDeptTree();
      // 选中后显示在输入框的值，即「父节点 / 子节点」
      addFullName(deptTree, 'label', ' / ');
      await formApi.updateSchema([
        {
          componentProps: () => ({
            className: 'w-full',
            fieldNames: {
              children: 'children',
              key: 'id',
              value: 'id',
            },
            getPopupContainer,
            onSelect: async (deptId: number | string) => {
              // 根据部门 ID 加载岗位
              await setupPostOptions(deptId);
              // 变化后需要重新选择岗位
              await formApi.setFieldValue('postIds', []);
            },
            placeholder: '请选择',
            showSearch: true,
            treeData: deptTree,
            treeDefaultExpandAll: true,
            treeLine: { showLeafIcon: false },
            // 筛选的字段
            treeNodeFilterProp: 'label',
            // 选中后显示在输入框的值
            treeNodeLabelProp: 'fullName',
          }),
          fieldName: 'deptId',
        },
      ]);
    }

    // 初始密码从系统参数读取（对应 Vue 侧 onMounted）
    useEffect(() => {
      let cancelled = false;
      void (async () => {
        const password = await configInfoByKey('sys.user.initPassword');
        if (!cancelled && password) {
          defaultPasswordRef.current = password;
        }
      })();
      return () => {
        cancelled = true;
      };
    }, []);

    /** 新增时候，从参数设置获取默认密码 */
    async function loadDefaultPassword(update: boolean) {
      if (!update && defaultPasswordRef.current) {
        await formApi.setFieldValue('password', defaultPasswordRef.current);
      }
    }

    const { markInitialized, onBeforeClose, resetInitialized } =
      useBeforeCloseDiff({
        currentGetter: defaultFormValueGetter(formApi),
        initializedGetter: defaultFormValueGetter(formApi),
      });

    const [BasicDrawer, drawerApi] = useWalnutDrawer({
      onBeforeClose,
      onClosed: handleClosed,
      onConfirm: handleConfirm,
      onOpenChange: async (isOpen) => {
        if (!isOpen) {
          // 需要重置岗位选择
          await formApi.updateSchema([
            {
              componentProps: {
                options: [],
                placeholder: '请先选择部门',
              },
              fieldName: 'postIds',
            },
          ]);
          return;
        }
        drawerApi.drawerLoading(true);

        const { id } = drawerApi.getData<{ id?: number | string }>();
        const updating = !!id;
        setUpdateFlag(updating);

        /** update 时禁用用户名修改、不显示密码框 */
        await formApi.updateSchema([
          { componentProps: { disabled: updating }, fieldName: 'userName' },
          {
            dependencies: {
              if: () => !updating,
              triggerFields: ['id'],
            },
            fieldName: 'password',
          },
        ]);

        // 更新 && 赋值
        const { postIds, posts, roleIds, roles, user } = await findUserInfo(id);
        const postOptions = (posts ?? []).map((item) => ({
          label: item.postName,
          value: item.id,
        }));
        await formApi.updateSchema([
          {
            componentProps: {
              // title 用于选中后回填到输入框，默认为 label
              optionLabelProp: 'title',
              options: (roles ?? []).map((item) => ({
                label: genRoleOptionLabel(item),
                // title 用于选中后回填到输入框，默认为 label
                title: item.roleName,
                value: item.id,
              })),
            },
            fieldName: 'roleIds',
          },
          {
            componentProps: { options: postOptions },
            fieldName: 'postIds',
          },
        ]);

        // 部门选择、初始密码及用户相关操作并行处理
        const promises: Promise<any>[] = [
          setupDeptSelect(),
          loadDefaultPassword(updating),
        ];
        if (user) {
          promises.push(
            // 添加基础信息
            formApi.setValues(user as unknown as Record<string, any>),
            // 添加角色和岗位
            formApi.setFieldValue('postIds', postIds),
            formApi.setFieldValue('roleIds', roleIds),
            // 更新时不会触发 onSelect，需要手动调用
            setupPostOptions(user.deptId),
          );
        }
        await Promise.all(promises);
        await markInitialized();

        drawerApi.drawerLoading(false);
      },
    });

    useImperativeHandle(
      ref,
      () => ({
        openAdd: () => {
          drawerApi.setData({});
          drawerApi.open();
        },
        openEdit: (id) => {
          drawerApi.setData({ id });
          drawerApi.open();
        },
      }),
      [drawerApi],
    );

    async function handleConfirm() {
      try {
        drawerApi.lock(true);
        const { valid } = await formApi.validate();
        if (!valid) return;
        const data = cloneDeep(await formApi.getValues());
        await (isUpdateRef.current
          ? userUpdate(data)
          : userAdd(data));
        resetInitialized();
        onReload();
        await drawerApi.close();
      } catch (error) {
        console.error(error);
      } finally {
        drawerApi.lock(false);
      }
    }

    async function handleClosed() {
      await formApi.resetForm();
      resetInitialized();
    }

    return (
      <BasicDrawer
        class="w-[600px]"
        title={isUpdate ? $t('pages.common.edit') : $t('pages.common.add')}
      >
        <BasicForm />
      </BasicDrawer>
    );
  },
);

export default UserDrawer;
