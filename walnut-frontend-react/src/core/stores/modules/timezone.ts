import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { DEFAULT_TIME_ZONE_OPTIONS } from '@/core/preferences';
import { getCurrentTimezone, setCurrentTimezone } from '@/core/shared/utils';
interface TimezoneHandler {
  getTimezone?: () => Promise<string | null | undefined>;
  setTimezone?: (timezone: string) => Promise<void>;
  getTimezoneOptions?: () => Promise<{ label: string; value: string }[]>;
}
let customHandler: TimezoneHandler = {};
export function setTimezoneHandler(handler: TimezoneHandler) {
  customHandler = handler;
}
export const useTimezoneStore = create<{
  timezone: string;
  initTimezone: () => Promise<void>;
  setTimezone: (timezone: string) => Promise<void>;
  getTimezoneOptions: () => Promise<{ label: string; value: string }[]>;
  $reset: () => void;
}>()(
  persist(
    (set, get) => ({
      timezone: getCurrentTimezone(),
      initTimezone: async () => {
        const value = await customHandler.getTimezone?.();
        if (value) set({ timezone: value });
        setCurrentTimezone(value || get().timezone);
      },
      setTimezone: async (timezone) => {
        await customHandler.setTimezone?.(timezone);
        setCurrentTimezone(timezone);
        set({ timezone });
      },
      getTimezoneOptions: async () =>
        customHandler.getTimezoneOptions
          ? customHandler.getTimezoneOptions()
          : DEFAULT_TIME_ZONE_OPTIONS.map((item) => ({ label: item.label, value: item.timezone })),
      $reset: () => set({ timezone: getCurrentTimezone() }),
    }),
    {
      skipHydration: true,
      name: 'core-timezone',
      partialize: (state) => ({ timezone: state.timezone }),
    },
  ),
);
