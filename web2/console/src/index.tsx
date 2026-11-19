import { render } from 'solid-js/web';
import '@/styles/index.css';
import App from './App';
import { ErrorBoundary } from '@/components/ErrorBoundary';
import { Toasts } from '@/lib/storage/toast';
import { ConfirmHost } from '@/lib/widgets';

const root = document.getElementById('root');
if (!root) throw new Error('root element not found');

// 首帧后再启用主题过渡，避免初始渲染时颜色渐变。
window.setTimeout(() => {
  document.documentElement.classList.add('transition-colors', 'duration-700');
  document.body.classList.add('transition-colors', 'duration-700');
}, 1000);

render(
  () => (
    <ErrorBoundary>
      <App />
      <Toasts />
      <ConfirmHost />
    </ErrorBoundary>
  ),
  root,
);
