import type { TabDefinition, TabMeta } from '@vben-core/typings';

import { updatePreferences } from '@vben-core/preferences';
import { Stack } from '@vben-core/shared/utils';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  getTabKey,
  selectAffixTabs,
  selectTabs,
  useTabbarStore,
} from '../tabbar';

const STORAGE_KEY = 'walnut-seed-admin-core-tabbar';

function makeTab(
  path: string,
  meta: TabMeta = {},
  extra: Partial<TabDefinition> = {},
): TabDefinition {
  return { fullPath: path, meta, name: path, path, ...extra };
}

/** 每次取最新快照：action 引用稳定，但 state 字段必须走 getState() */
const s = () => useTabbarStore.getState();

describe('useTabbarStore', () => {
  beforeEach(() => {
    useTabbarStore.getState().$reset();
    updatePreferences({ tabbar: { maxCount: 0, visitHistory: true } });
    sessionStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('addTab', () => {
    it('新页签直接追加，并补上推导出的 key', () => {
      const added = s().addTab(makeTab('/home'));

      expect(s().tabs).toHaveLength(1);
      expect(s().tabs[0]).toEqual(added);
      expect(added.key).toBe('/home');
    });

    it('同 key 页签去重，只更新参数而不新增', () => {
      s().addTab(makeTab('/detail', { fullPathKey: false }));
      s().addTab(
        makeTab('/detail', { fullPathKey: false }, { query: { id: '1' } }),
      );

      expect(s().tabs).toHaveLength(1);
      expect(s().tabs[0]?.query).toEqual({ id: '1' });
    });

    it('已存在的页签合并 meta，但保留原有的 affixTab 与 newTabTitle', () => {
      const first = s().addTab(makeTab('/home', { affixTab: true }));
      s().setTabTitle(first, '自定义标题');

      const merged = s().addTab(
        makeTab('/home', { affixTab: false, icon: 'home' }),
      );

      expect(s().tabs).toHaveLength(1);
      expect(merged.meta?.affixTab).toBe(true);
      expect(merged.meta?.newTabTitle).toBe('自定义标题');
      expect(merged.meta?.icon).toBe('home');
    });

    it('meta.hideInTab 或 matched 中有 hideInTab 时不进列表', () => {
      s().addTab(makeTab('/hidden', { hideInTab: true }));
      s().addTab(
        makeTab('/hidden-parent', undefined, {
          matched: [
            {
              meta: { hideInTab: true, title: 'Hidden' },
              path: '/hidden-parent',
            },
          ],
        }),
      );

      expect(s().tabs).toHaveLength(0);
    });

    it('克隆入参，不写穿调用方的对象', () => {
      const source = makeTab('/home', { title: 'Home' });
      s().addTab(source);

      expect(source.key).toBeUndefined();
      expect(s().tabs[0]).not.toBe(source);
      expect(s().tabs[0]?.meta).not.toBe(source.meta);
    });

    it('maxCount 溢出时关掉第一个非固定页签', () => {
      updatePreferences({ tabbar: { maxCount: 2 } });
      s().addTab(makeTab('/affix', { affixTab: true }));
      s().addTab(makeTab('/b'));
      s().addTab(makeTab('/c'));

      expect(s().tabs.map((tab) => tab.path)).toEqual(['/affix', '/c']);
    });

    it('maxCount = 0 表示不限制', () => {
      for (const path of ['/a', '/b', '/c', '/d']) {
        s().addTab(makeTab(path));
      }

      expect(s().tabs).toHaveLength(4);
    });

    it('maxNumOfOpenTab 限制同名路由的打开数', () => {
      const meta: TabMeta = { maxNumOfOpenTab: 2 };
      s().addTab(makeTab('/x/1', meta, { name: 'X' }));
      s().addTab(makeTab('/x/2', meta, { name: 'X' }));
      s().addTab(makeTab('/x/3', meta, { name: 'X' }));

      expect(s().tabs.map((tab) => tab.path)).toEqual(['/x/2', '/x/3']);
    });
  });

  describe('固定页签', () => {
    it('affixTabs 按 affixTabOrder 升序，getTabs 把固定页签排在最前', () => {
      s().addTab(makeTab('/normal'));
      s().addTab(makeTab('/affix-b', { affixTab: true, affixTabOrder: 2 }));
      s().addTab(makeTab('/affix-a', { affixTab: true, affixTabOrder: 1 }));

      expect(selectAffixTabs(s()).map((tab) => tab.path)).toEqual([
        '/affix-a',
        '/affix-b',
      ]);
      expect(selectTabs(s()).map((tab) => tab.path)).toEqual([
        '/affix-a',
        '/affix-b',
        '/normal',
      ]);
      expect(s().getTabs().map((tab) => tab.path)).toEqual([
        '/affix-a',
        '/affix-b',
        '/normal',
      ]);
    });

    it('affixTabOrder 缺省按 0 处理', () => {
      s().addTab(makeTab('/affix-b', { affixTab: true, affixTabOrder: 5 }));
      s().addTab(makeTab('/affix-a', { affixTab: true }));

      expect(s().affixTabs().map((tab) => tab.path)).toEqual([
        '/affix-a',
        '/affix-b',
      ]);
    });

    it('_close 不会关掉固定页签', () => {
      const affix = makeTab('/affix', { affixTab: true });
      s().addTab(affix);

      s()._close(affix);

      expect(s().tabs).toHaveLength(1);
    });

    it('closeTab 关不掉固定页签', () => {
      const affix = makeTab('/affix', { affixTab: true });
      s().addTab(affix);
      s().addTab(makeTab('/other'));

      s().closeTab(affix, '/affix');

      expect(s().tabs.map((tab) => tab.path)).toEqual(['/affix', '/other']);
    });

    it('pinTab 置 affixTab 并移到固定区，unpinTab 移回活动区首位', () => {
      s().addTab(makeTab('/affix', { affixTab: true }));
      s().addTab(makeTab('/a'));
      const b = s().addTab(makeTab('/b'));

      s().pinTab(b);
      expect(s().tabs.map((tab) => tab.path)).toEqual(['/affix', '/b', '/a']);

      const pinned = s().tabs[1]!;
      s().unpinTab(pinned);
      expect(s().tabs.map((tab) => tab.path)).toEqual(['/affix', '/b', '/a']);
      expect(s().tabs[1]?.meta?.affixTab).toBe(false);
    });

    it('toggleTabPin 在固定/取消固定之间切换', () => {
      const tab = s().addTab(makeTab('/a'));

      s().toggleTabPin(tab);
      expect(s().tabs[0]?.meta?.affixTab).toBe(true);

      s().toggleTabPin(s().tabs[0]!);
      expect(s().tabs[0]?.meta?.affixTab).toBe(false);
    });

    it('setAffixTabs 把路由批量转成固定页签', () => {
      s().setAffixTabs([
        { meta: { title: 'Home' }, name: 'Home', path: '/home' },
      ]);

      expect(s().tabs).toHaveLength(1);
      expect(s().tabs[0]?.meta?.affixTab).toBe(true);
      expect(s().tabs[0]?.key).toBe('/home');
    });
  });

  describe('closeTab', () => {
    it('关非当前页签：返回 null，不要求跳转', () => {
      s().addTab(makeTab('/a'));
      const b = s().addTab(makeTab('/b'));

      expect(s().closeTab(b, '/a')).toBeNull();
      expect(s().tabs.map((tab) => tab.path)).toEqual(['/a']);
    });

    it('未传 currentTabKey 时按「关的不是当前页」处理', () => {
      s().addTab(makeTab('/a'));
      const b = s().addTab(makeTab('/b'));

      expect(s().closeTab(b)).toBeNull();
      expect(s().tabs).toHaveLength(1);
    });

    it('关当前页签：开启访问历史时跳回上一个访问过的页签', () => {
      s().addTab(makeTab('/a'));
      s().addTab(makeTab('/b'));
      const c = s().addTab(makeTab('/c'));

      expect(s().closeTab(c, '/c')).toBe('/b');
      expect(s().visitHistory.toArray()).toEqual(['/a']);

      const b = s().tabs.find((tab) => tab.path === '/b')!;
      expect(s().closeTab(b, '/b')).toBe('/a');
    });

    it('关当前页签：历史栈顶指向已关闭的页签时继续往下弹', () => {
      s().addTab(makeTab('/a'));
      const b = s().addTab(makeTab('/b'));
      const c = s().addTab(makeTab('/c'));
      s()._close(b); // 直接摘除，不动访问历史

      expect(s().closeTab(c, '/c')).toBe('/a');
      expect(s().visitHistory.toArray()).toEqual([]);
    });

    it('关当前页签：访问历史里找不到上一个时退回第一个页签', () => {
      const a = s().addTab(makeTab('/a'));
      s().addTab(makeTab('/b'));
      s().visitHistory.clear();

      expect(s().closeTab(a, '/a')).toBe('/b');
    });

    it('关当前页签：未开启访问历史时跳下一个，没有下一个则跳上一个', () => {
      updatePreferences({ tabbar: { visitHistory: false } });
      s().addTab(makeTab('/a'));
      const b = s().addTab(makeTab('/b'));
      s().addTab(makeTab('/c'));

      expect(s().closeTab(b, '/b')).toBe('/c');
      expect(s().closeTab(s().tabs.find((t) => t.path === '/c')!, '/c')).toBe(
        '/a',
      );
    });

    it('只剩一个页签时拒绝关闭', () => {
      const error = vi.spyOn(console, 'error').mockImplementation(() => {});
      const a = s().addTab(makeTab('/a'));

      expect(s().closeTab(a, '/a')).toBeNull();
      expect(s().tabs).toHaveLength(1);
      expect(error).toHaveBeenCalled();
    });

    it('closeTabByKey 先 decode 再按 key 定位', () => {
      s().addTab(makeTab('/a'));
      s().addTab(makeTab('/b'));

      expect(s().closeTabByKey(encodeURIComponent('/a'), '/b')).toBeNull();
      expect(s().tabs.map((tab) => tab.path)).toEqual(['/b']);
    });

    it('closeTabByKey 关当前页签时返回跳转目标', () => {
      s().addTab(makeTab('/a'));
      s().addTab(makeTab('/b'));

      expect(s().closeTabByKey('/b', '/b')).toBe('/a');
    });

    it('closeTabByKey 找不到 key 时返回 null', () => {
      s().addTab(makeTab('/a'));

      expect(s().closeTabByKey('/nope', '/a')).toBeNull();
      expect(s().tabs).toHaveLength(1);
    });
  });

  describe('批量关闭', () => {
    beforeEach(() => {
      for (const path of ['/a', '/b', '/c', '/d']) {
        s().addTab(makeTab(path));
      }
    });

    it('closeLeftTabs 关掉左侧全部', () => {
      s().closeLeftTabs(s().tabs[2]!);

      expect(s().tabs.map((tab) => tab.path)).toEqual(['/c', '/d']);
    });

    it('closeLeftTabs 目标已在最左时不动', () => {
      s().closeLeftTabs(s().tabs[0]!);

      expect(s().tabs).toHaveLength(4);
    });

    it('closeRightTabs 关掉右侧全部', () => {
      s().closeRightTabs(s().tabs[1]!);

      expect(s().tabs.map((tab) => tab.path)).toEqual(['/a', '/b']);
    });

    it('closeRightTabs 目标已在最右时不动', () => {
      s().closeRightTabs(s().tabs[3]!);

      expect(s().tabs).toHaveLength(4);
    });

    it('closeOtherTabs 只留目标页签', () => {
      s().closeOtherTabs(s().tabs[1]!);

      expect(s().tabs.map((tab) => tab.path)).toEqual(['/b']);
    });

    it('closeOtherTabs 保留固定页签', () => {
      s().pinTab(s().tabs[0]!);

      s().closeOtherTabs(s().tabs.find((tab) => tab.path === '/c')!);

      expect(s().tabs.map((tab) => tab.path)).toEqual(['/a', '/c']);
    });

    it('closeAllTabs 无固定页签时保留第一个并返回其路径', () => {
      expect(s().closeAllTabs()).toBe('/a');
      expect(s().tabs.map((tab) => tab.path)).toEqual(['/a']);
    });

    it('closeAllTabs 有固定页签时只留固定页签', () => {
      s().pinTab(s().tabs[1]!);

      expect(s().closeAllTabs()).toBe('/b');
      expect(s().tabs.map((tab) => tab.path)).toEqual(['/b']);
    });

    it('_bulkCloseByKeys 按 key 批量摘除', () => {
      s()._bulkCloseByKeys(['/a', '/d']);

      expect(s().tabs.map((tab) => tab.path)).toEqual(['/b', '/c']);
    });
  });

  describe('getTabKey', () => {
    it('query.pageKey 优先级最高', () => {
      expect(
        getTabKey({
          fullPath: '/a?id=1',
          path: '/a',
          query: { pageKey: 'pk' },
        }),
      ).toBe('pk');
    });

    it('pageKey 为数组时取第一个', () => {
      expect(
        getTabKey({
          fullPath: '/a',
          path: '/a',
          query: { pageKey: ['pk1', 'pk2'] },
        }),
      ).toBe('pk1');
    });

    it('fullPathKey = false 时用 path', () => {
      expect(
        getTabKey({
          fullPath: '/a?id=1',
          meta: { fullPathKey: false },
          path: '/a',
        }),
      ).toBe('/a');
    });

    it('默认用 fullPath，缺失时退回 path', () => {
      expect(getTabKey({ fullPath: '/a?id=1', path: '/a' })).toBe('/a?id=1');
      expect(getTabKey({ path: '/a' })).toBe('/a');
    });

    it('结果做 decodeURIComponent，非法编码时原样返回', () => {
      expect(getTabKey({ fullPath: '/a?n=%E4%B8%AD', path: '/a' })).toBe(
        '/a?n=中',
      );
      expect(getTabKey({ fullPath: '/a/%E0%A4%A', path: '/a' })).toBe(
        '/a/%E0%A4%A',
      );
    });

    it('tab 上已有的 key 优先于重新推导', () => {
      const tab = makeTab('/a', {}, { key: 'custom' });
      s().addTab(tab);

      expect(s().getTabByKey('custom')?.path).toBe('/a');
      expect(s().getTabByKey('/a')).toBeUndefined();
    });
  });

  describe('visitHistory', () => {
    it('addTab 入栈，重复访问时去重并移到栈顶', () => {
      s().addTab(makeTab('/a'));
      s().addTab(makeTab('/b'));
      s().addTab(makeTab('/a'));

      expect(s().visitHistory.toArray()).toEqual(['/b', '/a']);
      expect(s().visitHistory.peek()).toBe('/a');
    });

    it('关闭页签时从栈里移除', () => {
      s().addTab(makeTab('/a'));
      const b = s().addTab(makeTab('/b'));

      s().closeTab(b, '/a');

      expect(s().visitHistory.toArray()).toEqual(['/a']);
    });

    it('closeAllTabs 用 retain 只保留剩下的页签', () => {
      s().addTab(makeTab('/a'));
      s().addTab(makeTab('/b'));
      s().addTab(makeTab('/c'));

      s().closeAllTabs();

      expect(s().visitHistory.toArray()).toEqual(['/a']);
    });

    it('关闭 visitHistory 偏好时不再入栈', () => {
      updatePreferences({ tabbar: { visitHistory: false } });
      s().addTab(makeTab('/a'));

      expect(s().visitHistory.size).toBe(0);
    });

    it('最多保留 50 条', () => {
      for (let i = 0; i < 60; i++) {
        s().addTab(makeTab(`/p/${i}`));
      }

      expect(s().visitHistory.size).toBe(50);
      expect(s().visitHistory.peek()).toBe('/p/59');
    });

    it('持久化后 visitHistory 退化为普通对象，rehydrate 时重建 Stack', async () => {
      s().addTab(makeTab('/a'));
      s().addTab(makeTab('/b'));

      const persisted = JSON.parse(sessionStorage.getItem(STORAGE_KEY)!);
      expect(persisted.state.visitHistory).not.toBeInstanceOf(Stack);
      expect(persisted.state.visitHistory.items).toEqual(['/a', '/b']);
      expect(persisted.state.visitHistory.maxSize).toBe(50);

      await useTabbarStore.persist.rehydrate();

      const revived = s().visitHistory;
      expect(revived).toBeInstanceOf(Stack);
      expect(revived.toArray()).toEqual(['/a', '/b']);
      expect(revived.peek()).toBe('/b');
    });

    it('没有历史数据时 rehydrate 得到空 Stack 而不是普通对象', async () => {
      sessionStorage.clear();

      await useTabbarStore.persist.rehydrate();

      expect(s().visitHistory).toBeInstanceOf(Stack);
      expect(s().visitHistory.size).toBe(0);
    });

    it('只持久化 tabs / visitHistory 两个字段', () => {
      s().addTab(makeTab('/a', { keepAlive: true }, { name: 'A' }));

      const persisted = JSON.parse(sessionStorage.getItem(STORAGE_KEY)!);
      expect(Object.keys(persisted.state).sort()).toEqual([
        'tabs',
        'visitHistory',
      ]);
    });
  });

  describe('缓存集合', () => {
    it('updateCacheTabs 收集 keepAlive 页签自身与 matched 的父级 name', () => {
      s().addTab(
        makeTab(
          '/a/b',
          { keepAlive: true },
          {
            matched: [
              { name: 'Root', path: '/' },
              { name: 'A', path: '/a' },
              { name: 'B', path: '/a/b' },
            ],
            name: 'B',
          },
        ),
      );
      s().addTab(makeTab('/plain', {}, { name: 'Plain' }));

      expect(s().getCachedTabs()).toEqual(['A', 'B']);
    });

    it('关掉页签后同步移出缓存集合', () => {
      const tab = s().addTab(
        makeTab('/a', { keepAlive: true }, { name: 'A' }),
      );
      expect(s().getCachedTabs()).toEqual(['A']);

      s()._close(tab);
      s().updateCacheTabs();

      expect(s().getCachedTabs()).toEqual([]);
    });

    it('refreshByName 期间把路由名放进 excludeCachedTabs', async () => {
      vi.useFakeTimers();
      try {
        const pending = s().refreshByName('A');
        expect(s().getExcludeCachedTabs()).toEqual(['A']);

        await vi.advanceTimersByTimeAsync(200);
        await pending;
      } finally {
        vi.useRealTimers();
      }

      expect(s().getExcludeCachedTabs()).toEqual([]);
    });

    it('refresh 额外把 renderRouteView 短暂置 false', async () => {
      vi.useFakeTimers();
      try {
        const pending = s().refresh('A');
        expect(s().renderRouteView).toBe(false);
        expect(s().getExcludeCachedTabs()).toEqual(['A']);

        await vi.advanceTimersByTimeAsync(200);
        await pending;
      } finally {
        vi.useRealTimers();
      }

      expect(s().renderRouteView).toBe(true);
      expect(s().getExcludeCachedTabs()).toEqual([]);
    });
  });

  describe('其余动作', () => {
    it('setTabTitle 写入 newTabTitle（支持取值函数）', () => {
      const tab = s().addTab(makeTab('/a', { title: 'A' }));

      s().setTabTitle(tab, () => '动态标题');

      expect(s().tabs[0]?.meta?.newTabTitle).toBeTypeOf('function');
      expect(s().tabs[0]?.meta?.title).toBe('A');
    });

    it('resetTabTitle 沿用 Vue 版行为：入参已有 newTabTitle 时直接 return', () => {
      const tab = s().addTab(makeTab('/a', { title: 'A' }));
      s().setTabTitle(tab, '自定义标题');

      s().resetTabTitle(s().tabs[0]!);
      expect(s().tabs[0]?.meta?.newTabTitle).toBe('自定义标题');

      // 只有本来就没有自定义标题时才走进清除分支（此时清除是幂等的）
      const plain = s().addTab(makeTab('/b', { title: 'B' }));
      s().resetTabTitle(plain);
      expect(s().tabs[1]?.meta?.newTabTitle).toBeUndefined();
    });

    it('sortTabs 移动位置并递增 dragEndIndex', () => {
      s().addTab(makeTab('/a'));
      s().addTab(makeTab('/b'));
      s().addTab(makeTab('/c'));

      s().sortTabs(0, 2);

      expect(s().tabs.map((tab) => tab.path)).toEqual(['/b', '/c', '/a']);
      expect(s().dragEndIndex).toBe(1);
    });

    it('sortTabs 下标越界时不动', () => {
      s().addTab(makeTab('/a'));

      s().sortTabs(5, 0);

      expect(s().dragEndIndex).toBe(0);
    });

    it('setMenuList / getMenuList', () => {
      expect(s().getMenuList()).toContain('close-all');

      s().setMenuList(['close']);

      expect(s().getMenuList()).toEqual(['close']);
    });

    it('setUpdateTime 刷新时间戳', () => {
      const before = s().updateTime;

      s().setUpdateTime();

      expect(s().updateTime).toBeGreaterThanOrEqual(before);
    });

    it('openTabInNewWindow 用 fullPath 打开新窗口', () => {
      const open = vi.spyOn(window, 'open').mockImplementation(() => null);

      s().openTabInNewWindow(makeTab('/a?id=1'));

      expect(open).toHaveBeenCalledOnce();
      expect(open.mock.calls[0]?.[0]).toContain('/a?id=1');
      expect(open.mock.calls[0]?.[1]).toBe('_blank');
    });

    it('$reset 回到初始状态（含全新的 Stack 实例）', () => {
      s().addTab(makeTab('/a'));
      const oldStack = s().visitHistory;

      s().$reset();

      expect(s().tabs).toEqual([]);
      expect(s().visitHistory.size).toBe(0);
      expect(s().visitHistory).not.toBe(oldStack);
      expect(s().getCachedTabs()).toEqual([]);
    });
  });
});
