import type { DrawerState } from '../drawer-api';

import { beforeEach, describe, expect, it, vi } from 'vitest';

import { DrawerApi } from '../drawer-api';

/**
 * 用例移植自 Vue 侧 `popup-ui/src/drawer/__tests__/drawer-api.test.ts`（11 例），
 * 并补了 React 侧新增/差异的部分。
 *
 * 与 Vue 版的差异：Vue 版 `vi.mock('@/core/shared/store')` 自己手搓了一个
 * Store 替身；React 侧直接用真实的 TanStack Store（`@walnut-core/shared/store`
 * 就是它的再导出），既少一层 mock 失真，也和 tabbar store 的测试口径一致。
 */
describe('drawerApi', () => {
  let drawerApi: DrawerApi;
  let drawerState: DrawerState;

  beforeEach(() => {
    drawerApi = new DrawerApi();
    drawerState = drawerApi.store.state;
  });

  it('默认状态与 Vue 侧 defaultState 逐项一致', () => {
    expect(drawerState.isOpen).toBe(false);
    expect(drawerState.cancelText).toBeUndefined();
    expect(drawerState.confirmText).toBeUndefined();
    expect(drawerState.placement).toBe('right');
    expect(drawerState.closable).toBe(true);
    expect(drawerState.closeOnClickModal).toBe(true);
    expect(drawerState.closeOnPressEscape).toBe(true);
    expect(drawerState.modal).toBe(true);
    expect(drawerState.header).toBe(true);
    expect(drawerState.footer).toBe(true);
    expect(drawerState.destroyOnClose).toBe(true);
    expect(drawerState.showCancelButton).toBe(true);
    expect(drawerState.showConfirmButton).toBe(true);
    expect(drawerState.submitting).toBe(false);
    expect(drawerState.loading).toBe(false);
    expect(drawerState.confirmDisabled).toBe(false);
    expect(drawerState.confirmLoading).toBe(false);
  });

  it('构造参数覆盖默认值', () => {
    const api = new DrawerApi({ placement: 'left', title: 'T', width: 1 } as any);
    expect(api.store.state.placement).toBe('left');
    expect(api.store.state.title).toBe('T');
  });

  it('open 打开抽屉，并清掉上一轮的 submitting', () => {
    drawerApi.lock(true);
    expect(drawerApi.store.state.submitting).toBe(true);
    drawerApi.open();
    expect(drawerApi.store.state.isOpen).toBe(true);
    expect(drawerApi.store.state.submitting).toBe(false);
  });

  it('close 在 onBeforeClose 允许时关闭', async () => {
    drawerApi.open();
    await drawerApi.close();
    expect(drawerApi.store.state.isOpen).toBe(false);
  });

  it('close 在 onBeforeClose 返回 false 时不关闭', async () => {
    const onBeforeClose = vi.fn(() => false);
    const api = new DrawerApi({ onBeforeClose });
    api.open();
    await api.close();
    expect(api.store.state.isOpen).toBe(true);
    expect(onBeforeClose).toHaveBeenCalled();
  });

  it('close 同时复位 submitting', async () => {
    drawerApi.open();
    drawerApi.lock(true);
    await drawerApi.close();
    expect(drawerApi.store.state.submitting).toBe(false);
  });

  it('提供 onCancel 时由回调接管，抽屉保持打开', () => {
    const onCancel = vi.fn();
    const api = new DrawerApi({ onCancel });
    api.open();
    api.onCancel();
    expect(onCancel).toHaveBeenCalled();
    // 关闭逻辑不在 onCancel 内（与 Vue 侧一致）
    expect(api.store.state.isOpen).toBe(true);
  });

  it('未提供 onCancel 时回落到 close', async () => {
    drawerApi.open();
    drawerApi.onCancel();
    // onCancel 内部是 void close()，close 里有一次 await，用 waitFor 等它落定
    await vi.waitFor(() => expect(drawerApi.store.state.isOpen).toBe(false));
  });

  it('setData / getData 走 sharedData.payload', () => {
    const testData = { key: 'value' };
    drawerApi.setData(testData);
    expect(drawerApi.getData()).toEqual(testData);
    expect(drawerApi.getData<{ key: string }>().key).toBe('value');
  });

  it('setData 返回 this，可链式调用', () => {
    expect(drawerApi.setData({ id: 1 })).toBe(drawerApi);
  });

  it('setState 支持对象形态', () => {
    drawerApi.setState({ title: 'New Title' });
    expect(drawerApi.store.state.title).toBe('New Title');
  });

  it('setState 支持函数形态', () => {
    drawerApi.setState((prev) => ({ ...prev, confirmText: 'Yes' }));
    expect(drawerApi.store.state.confirmText).toBe('Yes');
  });

  it('lock / unlock 切换 submitting，drawerLoading 切换 loading + confirmDisabled', () => {
    drawerApi.lock();
    expect(drawerApi.store.state.submitting).toBe(true);
    drawerApi.unlock();
    expect(drawerApi.store.state.submitting).toBe(false);

    drawerApi.drawerLoading(true);
    expect(drawerApi.store.state.loading).toBe(true);
    expect(drawerApi.store.state.confirmDisabled).toBe(true);
    drawerApi.drawerLoading(false);
    expect(drawerApi.store.state.loading).toBe(false);
    expect(drawerApi.store.state.confirmDisabled).toBe(false);
  });

  it('isOpen 变化时触发 onOpenChange', async () => {
    const onOpenChange = vi.fn();
    const api = new DrawerApi({ onOpenChange });
    api.open();
    expect(onOpenChange).toHaveBeenCalledWith(true);
    await api.close();
    expect(onOpenChange).toHaveBeenLastCalledWith(false);
  });

  it('isOpen 未变化时不触发 onOpenChange', () => {
    const onOpenChange = vi.fn();
    const api = new DrawerApi({ onOpenChange });
    api.setState({ title: 'x' });
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it('onClosed 仅在已关闭时回调，onOpened 仅在已打开时回调', () => {
    const onClosed = vi.fn();
    const onOpened = vi.fn();
    const api = new DrawerApi({ onClosed, onOpened });

    api.onOpened(); // 尚未打开
    expect(onOpened).not.toHaveBeenCalled();
    api.onClosed(); // 尚未打开 == 已关闭
    expect(onClosed).toHaveBeenCalledTimes(1);

    api.open();
    api.onOpened();
    expect(onOpened).toHaveBeenCalledTimes(1);
    api.onClosed(); // 已打开，不该回调
    expect(onClosed).toHaveBeenCalledTimes(1);
  });

  it('onConfirm 透传给回调', () => {
    const onConfirm = vi.fn();
    const api = new DrawerApi({ onConfirm });
    api.onConfirm();
    expect(onConfirm).toHaveBeenCalled();
  });

  it('updateCallbacks 用最新回调覆盖构造时的闭包', async () => {
    const stale = vi.fn(() => false);
    const fresh = vi.fn(() => true);
    const api = new DrawerApi({ onBeforeClose: stale });

    api.open();
    api.updateCallbacks({ onBeforeClose: fresh });
    await api.close();

    expect(stale).not.toHaveBeenCalled();
    expect(fresh).toHaveBeenCalled();
    expect(api.store.state.isOpen).toBe(false);
  });

  it('方法可解构使用（this 已绑定，替代 Vue 侧的 bindMethods）', async () => {
    const { getData, open, setData } = drawerApi;
    setData({ id: 7 });
    open();
    expect(getData()).toEqual({ id: 7 });
    expect(drawerApi.store.state.isOpen).toBe(true);
  });
});
