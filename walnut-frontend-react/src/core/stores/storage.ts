import SecureLS from "secure-ls";
import type { StateStorage } from "zustand/middleware";
type SecureStorage = {
  get: (key: string) => string | null;
  set: (key: string, value: string) => void;
  remove: (key: string) => void;
};
type SecureCtor = new (options: Record<string, unknown>) => SecureStorage;
const module = SecureLS as unknown as {
  default?: SecureCtor;
  SecureLS?: SecureCtor;
};
const Constructor =
  module.default ?? module.SecureLS ?? (SecureLS as unknown as SecureCtor);
let storage: StateStorage;
export function initStoreStorage(
  namespace: string,
  options: { production?: boolean; encryptionSecret?: string } = {},
) {
  const env = (
    import.meta as ImportMeta & {
      env?: { DEV?: boolean; VITE_APP_STORE_SECURE_KEY?: string };
    }
  ).env;
  if (!(options.production ?? env?.DEV === false)) {
    storage = localStorage;
    return;
  }
  const secure = new Constructor({
    encodingType: "aes",
    encryptionSecret:
      options.encryptionSecret ?? env?.VITE_APP_STORE_SECURE_KEY,
    isCompression: true,
    metaKey: `${namespace}-secure-meta`,
  });
  storage = {
    getItem: (key) => {
      try {
        return secure.get(key) || null;
      } catch {
        localStorage.removeItem(key);
        return null;
      }
    },
    setItem: (key, value) => secure.set(key, value),
    removeItem: (key) => secure.remove(key),
  };
}
export function getStoreStorage(): StateStorage {
  return storage ?? localStorage;
}
