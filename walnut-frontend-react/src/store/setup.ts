import { getStoreStorage } from '@/core/stores';
import { createJSONStorage } from 'zustand/middleware';
import { useNotifyStore } from './notify';
import { useTenantStore } from './tenant';
export function initAppStores(namespace: string) {
  useNotifyStore.persist.setOptions({
    name: `${namespace}-app-notify`,
    storage: createJSONStorage(getStoreStorage),
  });
  useTenantStore.persist.setOptions({
    name: `${namespace}-app-tenant`,
    storage: createJSONStorage(getStoreStorage),
  });
  void useNotifyStore.persist.rehydrate();
  void useTenantStore.persist.rehydrate();
}
