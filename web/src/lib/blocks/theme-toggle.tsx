import stars from './darkmode-stars.svg';
import { colorScheme, toggleColorScheme } from '@/lib/storage/theme';

/** 太阳/月亮切换按钮（ret2shell DarkmodeButton 同款，纯 CSS 动画）。 */
export function ThemeToggle() {
  return (
    <button
      type="button"
      class="darkmode-wrapper"
      data-color-scheme={colorScheme()}
      title="切换配色"
      aria-label="切换配色"
      onClick={toggleColorScheme}
    >
      <img
        class="darkmode-stars"
        src={stars}
        alt=""
        draggable={false}
        onContextMenu={() => false}
      />
      <span class="darkmode-button" />
    </button>
  );
}
