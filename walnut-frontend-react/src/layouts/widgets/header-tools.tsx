import { useEffect, useMemo, useState } from 'react';
import { Avatar, Badge, Button, Empty, Form, Input, Modal, Popover, Select, Space } from 'antd';
import { Bell, Globe, LockKeyhole, Search, Trash2 } from 'lucide-react';
import { WalnutIcon, WalnutIconButton } from '@/core/shadcn-ui';
import { $t } from '@/core/locales';
import { updatePreferences, usePreferences } from '@/core/preferences';
import { useAccessStore, useUserStore, useTimezoneStore } from '@/core/stores';
import { useNavigate } from 'react-router';
import type { MenuRecordRaw } from '@/core/typings';
import { getCurrentTimezone, setCurrentTimezone } from '@/core/shared/utils';
import { DEFAULT_TIME_ZONE_OPTIONS } from '@/core/preferences';
import { appConfig } from '@/lib/config';
import { getMessageApi } from '@/services/message';
import { useAuthStore } from '@/store/auth';
import { useNotifyStore } from '@/store/notify';

export function openGlobalSearch() {
  window.dispatchEvent(new Event('walnut:search'));
}
export function openLockScreen() {
  window.dispatchEvent(new Event('walnut:lock'));
}

export function SearchButton() {
  return (
    <button
      type="button"
      aria-label="global-search"
      className="mr-4 hidden h-8 items-center gap-2 rounded-full bg-accent px-2.5 text-xs text-muted-foreground md:flex"
      onClick={openGlobalSearch}
    >
      <Search className="size-4" />
      <span>{$t('ui.widgets.search.title')}</span>
      <kbd className="rounded bg-background px-1 text-[10px]">Ctrl K</kbd>
    </button>
  );
}
function NotificationButton() {
  const userId = String(useUserStore((s) => s.userInfo?.userId) ?? '0');
  const store = useNotifyStore();
  const notifications = store.notifications.filter((item) => item.userId === userId);
  const content = (
    <div className="w-80 max-w-[calc(100vw-48px)]">
      <div className="flex items-center justify-between border-b pb-2">
        <strong>{$t('ui.widgets.notifications')}</strong>
        <Button type="text" size="small" onClick={() => store.readAll(userId)}>
          {$t('ui.widgets.markAllAsRead')}
        </Button>
      </div>
      <div className="max-h-96 overflow-y-auto">
        {notifications.length ? (
          notifications.map((item) => (
            <div key={item.id} className="flex gap-2 border-b py-3">
              <button className="flex-1 text-left" onClick={() => store.read(item.id)}>
                <Badge dot={!item.isRead}>
                  <strong>{item.title}</strong>
                </Badge>
                <p className="mt-1 whitespace-pre-wrap break-words">{item.message}</p>
                <small className="text-muted-foreground">{item.date}</small>
              </button>
              <Button
                type="text"
                size="small"
                aria-label={$t('common.delete')}
                icon={<Trash2 className="size-3" />}
                onClick={() => store.remove(item.id)}
              />
            </div>
          ))
        ) : (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={$t('common.noData')} />
        )}
      </div>
      <Button
        block
        type="text"
        disabled={!notifications.length}
        onClick={() => store.clear(userId)}
      >
        {$t('ui.widgets.clearNotifications')}
      </Button>
    </div>
  );
  return (
    <Popover trigger="click" placement="bottomRight" content={content}>
      <Badge dot={notifications.some((item) => !item.isRead)} offset={[-8, 8]}>
        <WalnutIconButton aria-label="notifications">
          <Bell className="size-4" />
        </WalnutIconButton>
      </Badge>
    </Popover>
  );
}
function TimezoneButton() {
  const { preferences } = usePreferences();
  return (
    <Popover
      trigger="click"
      placement="bottomRight"
      title={$t('ui.widgets.timezone.setTimezone')}
      content={
        <Select
          aria-label={$t('ui.widgets.timezone.setTimezone')}
          showSearch
          style={{ width: 240 }}
          value={preferences.app.timezone}
          options={DEFAULT_TIME_ZONE_OPTIONS.map((item) => ({
            label: item.label,
            value: item.timezone,
          }))}
          onChange={(value) => {
            void useTimezoneStore.getState().setTimezone(value);
            updatePreferences({ app: { timezone: value } });
          }}
        />
      }
    >
      <WalnutIconButton aria-label="timezone">
        <Globe className="size-4" />
      </WalnutIconButton>
    </Popover>
  );
}
export function HeaderTools() {
  const { preferences } = usePreferences();
  return (
    <>
      {preferences.widget.notification && <NotificationButton />}
      {preferences.widget.lockScreen && (
        <WalnutIconButton aria-label="lock-screen" onClick={openLockScreen}>
          <LockKeyhole className="size-4" />
        </WalnutIconButton>
      )}
      {preferences.widget.timezone && <TimezoneButton />}
    </>
  );
}

/** Mounted above all shell variants, so connections, dialogs and keyboard listeners have one owner. */
export function ShellServices() {
  const { preferences } = usePreferences();
  const menus = useAccessStore((s) => s.accessMenus);
  const locked = useAccessStore((s) => s.isLockScreen);
  const token = useAccessStore((s) => s.accessToken);
  const user = useUserStore((s) => s.userInfo);
  const [searchOpen, setSearchOpen] = useState(false);
  const [lockOpen, setLockOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [selected, setSelected] = useState(0);
  const [now, setNow] = useState(new Date());
  const [lockForm] = Form.useForm();
  const [unlockForm] = Form.useForm();
  const navigate = useNavigate();
  const results = useMemo(() => {
    function walk(
      nodes: MenuRecordRaw[],
      parents: string[] = [],
    ): { name: string; path: string; icon?: string }[] {
      return nodes.flatMap((node) =>
        node.children?.length
          ? walk(node.children, [...parents, $t(node.name)])
          : [
              {
                name: [...parents, $t(node.name)].join(' / '),
                path: node.path,
                icon: typeof node.icon === 'string' ? node.icon : undefined,
              },
            ],
      );
    }
    return walk(menus)
      .filter((node) => node.name.toLowerCase().includes(query.toLowerCase()))
      .slice(0, 50);
  }, [menus, query, preferences.app.locale]);
  function select(index: number) {
    const target = results[index];
    if (!target) return;
    setSearchOpen(false);
    if (/^https?:/.test(target.path)) window.open(target.path, '_blank', 'noopener,noreferrer');
    else navigate(target.path);
  }
  useEffect(() => {
    const search = () => {
      setSearchOpen(true);
      setSelected(0);
      setQuery('');
    };
    const lock = () => setLockOpen(true);
    function keydown(e: KeyboardEvent) {
      if (locked || !preferences.shortcutKeys.enable) return;
      if (
        (e.ctrlKey || e.metaKey) &&
        e.key.toLowerCase() === 'k' &&
        preferences.shortcutKeys.globalSearch
      ) {
        e.preventDefault();
        search();
      }
      if (e.altKey && e.key.toLowerCase() === 'l' && preferences.shortcutKeys.globalLockScreen) {
        e.preventDefault();
        lock();
      }
    }
    window.addEventListener('walnut:search', search);
    window.addEventListener('walnut:lock', lock);
    window.addEventListener('keydown', keydown);
    return () => {
      window.removeEventListener('walnut:search', search);
      window.removeEventListener('walnut:lock', lock);
      window.removeEventListener('keydown', keydown);
    };
  }, [preferences.shortcutKeys, locked]);
  useEffect(() => {
    void useTimezoneStore.getState().setTimezone(preferences.app.timezone);
  }, [preferences.app.timezone]);
  useEffect(() => {
    document.documentElement.classList.toggle('screen-locked', locked);
    return () => document.documentElement.classList.remove('screen-locked');
  }, [locked]);
  useEffect(() => {
    if (!locked) return;
    const timer = setInterval(() => setNow(new Date()), 1000);
    return () => clearInterval(timer);
  }, [locked]);
  useEffect(() => {
    if (!appConfig.sseEnable || !token || !user) return;
    const params = new URLSearchParams({
      clientid: appConfig.clientId,
      Authorization: `Bearer ${token}`,
    });
    const source = new EventSource(`${appConfig.apiURL}/resource/sse?${params}`);
    let failures = 0;
    source.onerror = () => {
      if (++failures > 3) source.close();
    };
    source.onmessage = (event) => {
      failures = 0;
      if (!event.data) return;
      useNotifyStore
        .getState()
        .add({
          id: crypto.randomUUID(),
          userId: String(user.userId),
          title: $t('component.notice.title'),
          message: event.data,
          date: new Date().toLocaleString(),
          isRead: false,
        });
      getMessageApi()?.notification.success({ title: $t('component.notice.received'), description: event.data });
    };
    return () => source.close();
  }, [token, user?.userId]);
  useEffect(() => {
    if (
      !preferences.app.enableCheckUpdates ||
      preferences.app.checkUpdatesInterval <= 0 ||
      ['localhost', '127.0.0.1'].includes(location.hostname)
    )
      return;
    let baseline: string | null = null;
    let cancelled = false;
    const controller = new AbortController();
    async function check() {
      if (document.hidden) return;
      try {
        const response = await fetch(import.meta.env.BASE_URL, {
          method: 'HEAD',
          cache: 'no-cache',
          signal: controller.signal,
        });
        const tag = response.headers.get('etag') || response.headers.get('last-modified');
        if (cancelled || !tag) return;
        if (baseline && tag !== baseline)
          getMessageApi()?.modal.confirm({
            title: $t('ui.widgets.checkUpdatesTitle'),
            content: $t('ui.widgets.checkUpdatesDescription'),
            okText: $t('common.refresh'),
            cancelText: $t('common.cancel'),
            onOk: () => location.reload(),
          });
        baseline = tag;
      } catch {
        /* Offline checks resume at the next interval. */
      }
    }
    void check();
    const timer = setInterval(check, preferences.app.checkUpdatesInterval * 60000);
    return () => {
      cancelled = true;
      controller.abort();
      clearInterval(timer);
    };
  }, [preferences.app.enableCheckUpdates, preferences.app.checkUpdatesInterval]);
  return (
    <>
      <Modal
        title={$t('ui.widgets.search.title')}
        open={searchOpen}
        footer={null}
        onCancel={() => setSearchOpen(false)}
        afterOpenChange={(open) => {
          if (!open) setQuery('');
        }}
      >
        <Input
          autoFocus
          prefix={<Search className="size-4" />}
          aria-label={$t('ui.widgets.search.searchNavigate')}
          placeholder={$t('ui.widgets.search.searchNavigate')}
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setSelected(0);
          }}
          onKeyDown={(e) => {
            if (e.key === 'ArrowDown') {
              e.preventDefault();
              setSelected(Math.min(selected + 1, results.length - 1));
            }
            if (e.key === 'ArrowUp') {
              e.preventDefault();
              setSelected(Math.max(selected - 1, 0));
            }
            if (e.key === 'Enter') select(selected);
          }}
        />
        <div role="listbox" className="mt-3 max-h-80 overflow-y-auto">
          {results.length ? (
            results.map((result, i) => (
              <button
                key={result.path}
                role="option"
                aria-selected={i === selected}
                className={`flex w-full items-center gap-3 rounded p-3 text-left ${i === selected ? 'bg-accent text-primary' : 'hover:bg-accent'}`}
                onClick={() => select(i)}
              >
                {result.icon && <WalnutIcon icon={result.icon} />}
                <span>{result.name}</span>
              </button>
            ))
          ) : (
            <Empty description={$t('ui.widgets.search.noResults')} />
          )}
        </div>
      </Modal>
      <Modal
        title={$t('ui.widgets.lockScreen.title')}
        open={lockOpen}
        onCancel={() => setLockOpen(false)}
        onOk={async () => {
          const { password } = await lockForm.validateFields();
          useAccessStore.getState().lockScreen(password);
          lockForm.resetFields();
          setLockOpen(false);
        }}
      >
        <Form form={lockForm} layout="vertical">
          <Form.Item
            name="password"
            label={$t('authentication.password')}
            rules={[{ required: true, message: $t('ui.widgets.lockScreen.placeholder') }]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
        </Form>
      </Modal>
      {locked && (
        <div
          className="fixed inset-0 z-[10000] flex flex-col items-center justify-center gap-5 bg-background text-foreground"
          role="dialog"
          aria-modal="true"
          aria-label={$t('ui.widgets.lockScreen.title')}
        >
          <div className="text-6xl font-semibold tabular-nums">
            {now.toLocaleTimeString(preferences.app.locale, {
              timeZone: preferences.app.timezone,
              hour: '2-digit',
              minute: '2-digit',
            })}
          </div>
          <p>
            {now.toLocaleDateString(preferences.app.locale, {
              timeZone: preferences.app.timezone,
              weekday: 'long',
              year: 'numeric',
              month: 'long',
              day: 'numeric',
            })}
          </p>
          <Avatar src={user?.avatar || preferences.app.defaultAvatar} size={80} />
          <strong>{user?.realName}</strong>
          <Form
            form={unlockForm}
            className="w-72"
            onFinish={({ password }) => {
              if (password === useAccessStore.getState().lockScreenPassword) {
                useAccessStore.getState().unlockScreen();
                unlockForm.resetFields();
              } else unlockForm.setFields([{ name: 'password', errors: [$t('authentication.passwordErrorTip')] }]);
            }}
          >
            <Form.Item name="password" rules={[{ required: true, message: $t('authentication.passwordTip') }]}>
              <Input.Password autoFocus aria-label={$t('authentication.password')} placeholder={$t('ui.widgets.lockScreen.placeholder')} />
            </Form.Item>
            <Button block type="primary" htmlType="submit">
              {$t('ui.widgets.lockScreen.screenButton')}
            </Button>
          </Form>
          <Button type="link" onClick={() => void useAuthStore.getState().logout()}>
            {$t('ui.widgets.lockScreen.backToLogin')}
          </Button>
        </div>
      )}
    </>
  );
}
