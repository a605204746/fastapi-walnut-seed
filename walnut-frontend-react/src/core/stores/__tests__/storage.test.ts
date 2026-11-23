import { afterEach, describe, expect, it } from "vitest";
import { getStoreStorage, initStoreStorage } from "../storage";
afterEach(() => {
  localStorage.clear();
  initStoreStorage("test");
});
describe("production persisted store adapter", () => {
  it("encrypts stored JSON and restores the exact serialized state", () => {
    initStoreStorage("encryption-test", {
      production: true,
      encryptionSecret: "test-only-secret",
    });
    const json = JSON.stringify({
      state: { accessToken: "test-only-token" },
      version: 1,
    });
    const storage = getStoreStorage();
    storage.setItem("encryption-test-core-access", json);
    expect(localStorage.getItem("encryption-test-core-access")).not.toContain(
      "test-only-token",
    );
    expect(storage.getItem("encryption-test-core-access")).toBe(json);
    storage.removeItem("encryption-test-core-access");
    expect(storage.getItem("encryption-test-core-access")).toBeNull();
  });
  it("retains plain localStorage during development", () => {
    initStoreStorage("dev-test", { production: false });
    getStoreStorage().setItem("dev-test-core-access", "serialized");
    expect(localStorage.getItem("dev-test-core-access")).toBe("serialized");
  });
});
