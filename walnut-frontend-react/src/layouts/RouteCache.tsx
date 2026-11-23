import { Activity, useContext, useRef } from 'react';
import { useLocation, useMatches, useOutlet, UNSAFE_LocationContext } from 'react-router';
import { getTabKey, useTabbarStore } from '@/core/stores';
import { usePreferences } from '@/core/preferences';
import { mergeMatchMeta } from '@/router/route-tree';
import { useUiStore } from '@/store/ui';

/** Keep only visited, open keepAlive tabs. Activity pauses effects of hidden pages. */
export function RouteCache() {
  const outlet = useOutlet();
  const location = useLocation();
  const locationContext = useContext(UNSAFE_LocationContext);
  const matches = useMatches();
  const meta = mergeMatchMeta(matches);
  const tabs = useTabbarStore((s) => s.tabs);
  const renderRouteView = useTabbarStore((s) => s.renderRouteView);
  const excluded = useTabbarStore((s) => s.excludeCachedTabs);
  const revisions = useUiStore((s) => s.revisionByPath);
  const { preferences } = usePreferences();
  const entries = useRef(
    new Map<
      string,
      { outlet: typeof outlet; context: typeof locationContext; revision: number; name?: string }
    >(),
  );
  const key = location.pathname + location.search;
  const name = (matches.at(-1)?.handle as { name?: string })?.name;
  const open = new Set(tabs.map(getTabKey));
  for (const cachedKey of entries.current.keys()) {
    if (
      (!open.has(cachedKey) && cachedKey !== key) ||
      excluded.has(entries.current.get(cachedKey)?.name ?? '')
    )
      entries.current.delete(cachedKey);
  }
  if (!preferences.tabbar.keepAlive) entries.current.clear();
  if (
    meta?.keepAlive &&
    preferences.tabbar.keepAlive &&
    renderRouteView &&
    !excluded.has(name ?? '')
  ) {
    const revision = revisions[key] ?? 0;
    entries.current.delete(key); // insertion order is the LRU order
    entries.current.set(key, { outlet, context: locationContext, revision, name });
  } else entries.current.delete(key);
  const max = preferences.tabbar.maxCount || 30;
  while (entries.current.size > max) entries.current.delete(entries.current.keys().next().value!);
  const cached = entries.current.has(key);
  return (
    <>
      {[...entries.current].map(([path, entry]) => (
        <Activity key={`${path}:${entry.revision}`} mode={path === key ? 'visible' : 'hidden'}>
          <UNSAFE_LocationContext.Provider value={entry.context}>
            <div className="h-full overflow-y-auto" data-cached-route={path}>
              {entry.outlet}
            </div>
          </UNSAFE_LocationContext.Provider>
        </Activity>
      ))}
      {!cached && renderRouteView ? (
        <div key={`${key}:${revisions[key] ?? 0}`} className="h-full overflow-y-auto">
          {outlet}
        </div>
      ) : null}
    </>
  );
}
