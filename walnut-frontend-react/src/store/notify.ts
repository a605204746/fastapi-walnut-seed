import { create } from 'zustand';
import { persist } from 'zustand/middleware';
export interface NotificationItem {
  id: string;
  userId: string;
  title: string;
  message: string;
  date: string;
  isRead: boolean;
}
export const useNotifyStore = create<{
  notifications: NotificationItem[];
  add: (item: NotificationItem) => void;
  read: (id: string) => void;
  readAll: (userId: string) => void;
  remove: (id: string) => void;
  clear: (userId: string) => void;
}>()(
  persist(
    (set) => ({
      notifications: [],
      add: (item) => set((s) => ({ notifications: [item, ...s.notifications].slice(0, 500) })),
      read: (id) =>
        set((s) => ({
          notifications: s.notifications.map((item) =>
            item.id === id ? { ...item, isRead: true } : item,
          ),
        })),
      readAll: (userId) =>
        set((s) => ({
          notifications: s.notifications.map((item) =>
            item.userId === userId ? { ...item, isRead: true } : item,
          ),
        })),
      remove: (id) =>
        set((s) => ({ notifications: s.notifications.filter((item) => item.id !== id) })),
      clear: (userId) =>
        set((s) => ({ notifications: s.notifications.filter((item) => item.userId !== userId) })),
    }),
    { skipHydration: true, name: 'walnut-react-notifications' },
  ),
);
