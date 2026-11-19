import type { MenuProps } from 'antd';
import type { WheelEvent as ReactWheelEvent } from 'react';

import type { RouteHandle, TabDefinition } from '@vben-core/typings';

import { useEffect, useRef } from 'react';

import { usePreferences } from '@vben/preferences';
import { getTabKey, useTabbarStore } from '@vben/stores';
import { VbenIcon, VbenIconButton } from '@vben-core/shadcn-ui';
import { Dropdown, Tabs } from 'antd';
import { Ellipsis, Maximize2, Minimize2, RotateCw, Pin } from 'lucide-react';
import { useLocation, useMatches, useNavigate } from 'react-router';
import { useShallow } from 'zustand/react/shallow';

import { $t } from '@/locales';
import { mergeMatchMeta } from '@/router/route-tree';
import { useUiStore } from '@/store/ui';

import './tabs.css';

/** 把当前 location + 匹配链组装成 store 需要的 TabDefinition */
function useCurrentTab(): TabDefinition {
  const location = useLocation();
  const matches = useMatches();

  const leaf = matches.at(-1);
  const handle = leaf?.handle as RouteHandle | undefined;

  return {
    fullPath: `${location.pathname}${location.search}`,
    matched: matches.map((match) => {
      const matchedHandle = match.handle as RouteHandle | undefined;
      return {
        meta: matchedHandle?.meta,
        name: matchedHandle?.name,
        path: match.pathname,
      };
    }),
    meta: mergeMatchMeta(matches),
    name: handle?.name,
    params: leaf?.params as Record<string, string>,
    path: location.pathname,
    query: Object.fromEntries(new URLSearchParams(location.search)),
  };
}

function tabTitle(tab: TabDefinition): string {
  const { meta, name } = tab;
  return (meta?.newTabTitle as string) || (meta?.title ? $t(meta.title) : '') || name || tab.path;
}

/**
 * 多页签栏：支持缓存刷新、固定、拖拽、滚轮、中键关闭和右键菜单。
 * store 管理打开顺序及访问历史，RouteCache 管理页面实例的生命周期。
 */
export function VbenTabs() {
  const navigate = useNavigate();
  const { preferences } = usePreferences();
  const currentTab = useCurrentTab();

  const addTab = useTabbarStore((state) => state.addTab);
  const closeAllTabs = useTabbarStore((state) => state.closeAllTabs);
  const closeLeftTabs = useTabbarStore((state) => state.closeLeftTabs);
  const closeOtherTabs = useTabbarStore((state) => state.closeOtherTabs);
  const closeRightTabs = useTabbarStore((state) => state.closeRightTabs);
  const closeTab = useTabbarStore((state) => state.closeTab);
  const openTabInNewWindow = useTabbarStore((state) => state.openTabInNewWindow);
  const toggleTabPin = useTabbarStore((state) => state.toggleTabPin);
  const sortTabs = useTabbarStore((state) => state.sortTabs);
  const refresh = useUiStore((state) => state.refresh);
  const maximized = useUiStore((state) => state.maximized);
  const toggleMaximized = useUiStore((state) => state.toggleMaximized);
  const dragIndexRef = useRef<null | number>(null);
  const navRef = useRef<HTMLDivElement>(null);

  // selectTabs 每次返回新数组，必须用 useShallow 做浅比较，否则任何 store 变化都会重渲染
  const tabs = useTabbarStore(useShallow((state) => state.getTabs()));

  const activeKey = getTabKey(currentTab);

  // 路由变化即登记页签；hideInTab 的过滤在 store 的 addTab 内部完成
  useEffect(() => {
    addTab(currentTab);
    // 只按「地址 + 匹配到的路由名」触发：currentTab 每次渲染都是新对象，放进依赖会死循环。
    // 带上 name 是因为同一个地址在动态路由注入前后会匹配到不同路由（`*` 兜底 → 真实页面），
    // 只看 fullPath 会漏掉这次变化。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentTab.fullPath, currentTab.name]);

  // `tabbar.persist` 为 false 时丢掉 sessionStorage 水合回来的页签，只留当前这个。
  // 与 Vue 侧 tabbar.vue 挂载时的处理一致（该偏好不控制 store 的 persist 本身）。
  useEffect(() => {
    if (preferences.tabbar.persist) {
      return;
    }
    closeOtherTabs(currentTab);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // `tabbar.maxCount`：超出上限时关掉最旧的非常驻、非当前页签
  useEffect(() => {
    const max = preferences.tabbar.maxCount;
    if (!max || max <= 0) return;
    const list = useTabbarStore.getState().getTabs();
    if (list.length <= max) return;
    const victim = list.find(
      (tab) => !tab.meta?.affixTab && getTabKey(tab) !== activeKey,
    );
    if (victim) closeTab(victim, activeKey);
  }, [tabs.length, preferences.tabbar.maxCount, activeKey, closeTab]);

  /** 滚轮横滚（tabbar.wheelable） */
  function handleWheel(e: ReactWheelEvent) {
    if (!preferences.tabbar.wheelable) return;
    const wrap = navRef.current?.querySelector(
      '.ant-tabs-nav-wrap',
    ) as HTMLElement | null;
    if (wrap) wrap.scrollLeft += e.deltaY;
  }

  function handleDragStart(index: number) {
    dragIndexRef.current = index;
  }

  function handleDrop(index: number) {
    const from = dragIndexRef.current;
    dragIndexRef.current = null;
    if (from === null || from === index) return;
    sortTabs(from, index);
  }

  function goToTab(key: string) {
    if (key !== activeKey) {
      navigate(key);
    }
  }

  function handleClose(tab: TabDefinition) {
    const target = closeTab(tab, activeKey);
    if (target) {
      navigate(target);
    }
  }

  function contextMenuItems(tab: TabDefinition): MenuProps['items'] {
    const affix = tab.meta?.affixTab ?? false;
    return [
      { key: 'close', label: $t('preferences.tabbar.contextMenu.close') },
      {
        key: 'affix',
        label: affix
          ? $t('preferences.tabbar.contextMenu.unpin')
          : $t('preferences.tabbar.contextMenu.pin'),
      },
      { key: 'reload', label: $t('preferences.tabbar.contextMenu.reload') },
      { key: 'maximize', label: $t('preferences.tabbar.contextMenu.maximize') },
      { type: 'divider' },
      {
        key: 'open-in-new-window',
        label: $t('preferences.tabbar.contextMenu.openInNewWindow'),
      },
      { type: 'divider' },
      {
        key: 'close-left',
        label: $t('preferences.tabbar.contextMenu.closeLeft'),
      },
      {
        key: 'close-right',
        label: $t('preferences.tabbar.contextMenu.closeRight'),
      },
      {
        key: 'close-other',
        label: $t('preferences.tabbar.contextMenu.closeOther'),
      },
      {
        key: 'close-all',
        label: $t('preferences.tabbar.contextMenu.closeAll'),
      },
    ];
  }

  function handleContextClick(key: string, tab: TabDefinition) {
    switch (key) {
      case 'reload': {
        if (getTabKey(tab) === activeKey) refresh();
        else void useTabbarStore.getState().refreshByName(tab.name ?? tab.path);
        break;
      }
      case 'maximize': { navigate(getTabKey(tab)); toggleMaximized(); break; }
      case 'affix': {
        toggleTabPin(tab);
        break;
      }
      case 'close': {
        handleClose(tab);
        break;
      }
      case 'close-all': {
        const target = closeAllTabs();
        if (target) {
          navigate(target);
        }
        break;
      }
      case 'close-left': {
        closeLeftTabs(tab);
        break;
      }
      case 'close-other': {
        closeOtherTabs(tab);
        break;
      }
      case 'close-right': {
        closeRightTabs(tab);
        break;
      }
      case 'open-in-new-window': {
        openTabInNewWindow(tab);
        break;
      }
    }
  }

  return (
    <div
      className={`vben-tabs vben-tabs--${preferences.tabbar.styleType} bg-header border-border flex shrink-0 items-center border-b`}
      onWheel={handleWheel}
      ref={navRef}
      style={{ '--tabbar-height': `${preferences.tabbar.height}px`, height: preferences.tabbar.height } as never}
    >
      <div className="min-w-0 flex-1">
      <Tabs
        activeKey={activeKey}
        hideAdd
        items={tabs.map((tab, index) => {
          const key = tab.key ?? getTabKey(tab);
          const affix = tab.meta?.affixTab ?? false;
          return {
            closable: !affix,
            key,
            label: (
              <Dropdown
                menu={{
                  items: contextMenuItems(tab),
                  onClick: ({ key: action }) => handleContextClick(action, tab),
                }}
                trigger={['contextMenu']}
              >
                <span
                  className="flex items-center gap-1"
                  draggable={preferences.tabbar.draggable}
                  onDragOver={(e) => e.preventDefault()}
                  onDragStart={() => handleDragStart(index)}
                  onDrop={() => handleDrop(index)}
                  onMouseDown={(e) => {
                    // 中键关闭（tabbar.middleClickToClose）
                    if (e.button === 1 && preferences.tabbar.middleClickToClose) {
                      e.preventDefault();
                      handleClose(tab);
                    }
                  }}
                >
                  {preferences.tabbar.showIcon && tab.meta?.icon ? (
                    <VbenIcon className="size-3.5" icon={tab.meta.icon} />
                  ) : null}
                  <span>{tabTitle(tab)}</span>{affix ? <Pin className="size-3" /> : null}
                </span>
              </Dropdown>
            ),
          };
        })}
        onChange={goToTab}
        onEdit={(targetKey, action) => {
          if (action !== 'remove') {
            return;
          }
          const key = typeof targetKey === 'string' ? targetKey : '';
          const tab = tabs.find((item) => (item.key ?? getTabKey(item)) === key);
          if (tab) {
            handleClose(tab);
          }
        }}
        size="small"
        type="editable-card"
      />
      </div>
      {/* 页签栏右侧工具：刷新 / 更多 / 最大化（tabbar.showRefresh / showMore / showMaximize） */}
      <div className="flex shrink-0 items-center gap-0.5 px-1.5">
        {preferences.tabbar.showRefresh ? (
          <VbenIconButton onClick={refresh}>
            <RotateCw className="size-4" />
          </VbenIconButton>
        ) : null}
        {preferences.tabbar.showMore ? (
          <Dropdown
            menu={{
              items: [
                {
                  key: 'close-other',
                  label: $t('preferences.tabbar.contextMenu.closeOther'),
                },
                {
                  key: 'close-all',
                  label: $t('preferences.tabbar.contextMenu.closeAll'),
                },
              ],
              onClick: ({ key }) => {
                if (key === 'close-all') {
                  const target = closeAllTabs();
                  if (target) navigate(target);
                } else {
                  closeOtherTabs(currentTab);
                }
              },
            }}
            trigger={['click']}
          >
            <VbenIconButton>
              <Ellipsis className="size-4" />
            </VbenIconButton>
          </Dropdown>
        ) : null}
        {preferences.tabbar.showMaximize ? (
          <VbenIconButton aria-label="maximize-content" onClick={toggleMaximized}>
            {maximized ? (
              <Minimize2 className="size-4" />
            ) : (
              <Maximize2 className="size-4" />
            )}
          </VbenIconButton>
        ) : null}
      </div>
    </div>
  );
}
