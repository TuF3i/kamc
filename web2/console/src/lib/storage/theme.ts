import { createEffect, createRoot, createSignal } from 'solid-js';

export type ColorScheme = 'dark' | 'light';

const STORAGE_KEY = 'kamc_theme';
const SWITCH_CLASS = 'theme-switching';
/** 主题切换过渡时长（与 tokens.css 的 .theme-switching 规则保持一致） */
const SWITCH_MS = 300;

function readInitial(): ColorScheme {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved === 'dark' || saved === 'light') return saved;
  } catch {
    // 隐私模式忽略
  }
  try {
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  } catch {
    return 'dark';
  }
}

function makeThemeRoot() {
  const [colorScheme, setColorSchemeSignal] = createSignal<ColorScheme>(readInitial());
  let switchTimer: number | undefined;

  createEffect(() => {
    const scheme = colorScheme();
    const root = document.documentElement;
    // 已有主题且发生切换：临时给全元素挂统一颜色过渡（首屏不加，避免初始渐变）。
    // 期间结束移除，不污染按钮悬停等自身交互过渡。
    if (root.getAttribute('data-theme') && root.getAttribute('data-theme') !== `kamc-${scheme}`) {
      root.classList.add(SWITCH_CLASS);
      window.clearTimeout(switchTimer);
      switchTimer = window.setTimeout(() => root.classList.remove(SWITCH_CLASS), SWITCH_MS + 60);
    }
    root.setAttribute('data-theme', `kamc-${scheme}`);
    try {
      localStorage.setItem(STORAGE_KEY, scheme);
    } catch {
      // 隐私模式忽略
    }
  });

  return { colorScheme, setColorSchemeSignal };
}

const themeRoot = createRoot(makeThemeRoot);

export const colorScheme = themeRoot.colorScheme;

export function setColorScheme(scheme: ColorScheme): void {
  themeRoot.setColorSchemeSignal(scheme);
}

export function toggleColorScheme(): void {
  themeRoot.setColorSchemeSignal(colorScheme() === 'dark' ? 'light' : 'dark');
}
