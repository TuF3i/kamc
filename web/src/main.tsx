import React from 'react';
import ReactDOM from 'react-dom/client';
import { App as AntApp, ConfigProvider } from 'antd';
import zhCN from 'antd/es/locale/zh_CN';
import dayjs from 'dayjs';
import 'dayjs/locale/zh-cn';
import '@xterm/xterm/css/xterm.css';
import '@/styles/global.css';
import App from '@/App';
import { themeConfig } from '@/theme';

dayjs.locale('zh-cn');

const root = document.getElementById('root');
if (!root) {
  throw new Error('root element not found');
}

ReactDOM.createRoot(root).render(
  <React.StrictMode>
    <ConfigProvider locale={zhCN} theme={themeConfig}>
      <AntApp>
        <App />
      </AntApp>
    </ConfigProvider>
  </React.StrictMode>,
);
