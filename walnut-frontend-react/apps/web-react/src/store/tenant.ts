import { create } from 'zustand';
import { persist } from 'zustand/middleware';
export const useTenantStore = create<{
  selected?: string;
  setSelected: (value?: string) => void;
}>()(
  persist((set) => ({ selected: undefined, setSelected: (selected) => set({ selected }) }), {
    skipHydration: true,
    name: 'walnut-react-dynamic-tenant',
  }),
);
