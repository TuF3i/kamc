import { render } from 'solid-js/web';
import '@/styles/index.css';
import App from './App';
import { ErrorBoundary } from '@/components/ErrorBoundary';
import { Toasts } from '@/lib/storage/toast';
import { ConfirmHost } from '@/lib/widgets';

const root = document.getElementById('root');
if (!root) throw new Error('root element not found');

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
