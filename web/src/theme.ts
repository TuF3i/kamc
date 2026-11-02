import { theme as antdTheme, type ThemeConfig } from 'antd';

export const themeConfig: ThemeConfig = {
  algorithm: antdTheme.darkAlgorithm,
  token: {
    colorPrimary: '#4c9aff',
    colorInfo: '#4c9aff',
    colorSuccess: '#34d399',
    colorWarning: '#fbbf24',
    colorError: '#f87171',
    borderRadius: 6,
    colorBgBase: '#0a111c',
    colorBgLayout: '#0a111c',
    colorBgContainer: '#101c2c',
    colorBgElevated: '#142236',
    colorBorder: '#1e2e44',
    colorBorderSecondary: '#182739',
    colorText: '#dbe7f5',
    colorTextSecondary: '#8aa0b8',
    colorTextTertiary: '#6f849d',
    colorTextQuaternary: '#51647a',
    fontFamily:
      "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
  },
  components: {
    Layout: {
      headerBg: '#0d1826',
      siderBg: '#070d17',
      bodyBg: '#0a111c',
      headerHeight: 56,
      headerPadding: '0 24px',
    },
    Menu: {
      darkItemBg: '#070d17',
      darkSubMenuItemBg: '#050a12',
      darkItemColor: '#8aa0b8',
      darkItemHoverBg: 'rgba(76, 154, 255, 0.10)',
      darkItemHoverColor: '#dbe7f5',
      darkItemSelectedBg: 'rgba(76, 154, 255, 0.18)',
      darkItemSelectedColor: '#8fc0ff',
      itemHeight: 42,
    },
    Card: {
      colorBgContainer: '#101c2c',
      borderRadiusLG: 8,
    },
    Table: {
      headerBg: '#0d1826',
      headerColor: '#8aa0b8',
      rowHoverBg: '#142236',
      borderColor: '#1a2a3e',
    },
    Button: {
      controlHeight: 34,
    },
    Tabs: {
      titleFontSize: 14,
    },
    Input: {
      colorBgContainer: '#0d1826',
    },
    Select: {
      colorBgContainer: '#0d1826',
    },
    Modal: {
      contentBg: '#101c2c',
      headerBg: '#101c2c',
    },
    Alert: {
      colorInfoBg: '#0e2036',
      colorInfoBorder: '#1e4d7a',
    },
  },
};
