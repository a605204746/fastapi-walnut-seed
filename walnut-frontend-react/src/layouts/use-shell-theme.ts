import { useAntdDesignTokens } from '@/core/hooks';
import { theme } from 'antd';

/** Local dark tokens must override the light tokens inherited from the app provider. */
export function useShellTheme() {
  const token = useAntdDesignTokens();
  return {
    algorithm: theme.darkAlgorithm,
    token: {
      colorBgBase: '#1c1e22',
      colorBgContainer: '#1c1e22',
      colorBgElevated: '#242424',
      colorBorder: '#363636',
      colorBorderSecondary: '#363636',
      colorTextBase: '#f2f2f2',
    },
    components: {
      Layout: { headerBg: '#1c1e22', siderBg: '#1c1e22' },
      Menu: {
        itemBg: 'transparent',
        itemColor: '#f2f2f2',
        itemSelectedBg: 'hsl(var(--primary) / 0.15)',
        itemSelectedColor: token.colorPrimary,
        subMenuItemBg: 'transparent',
      },
    },
  };
}
