import { createEffect, createRoot, createSignal } from 'solid-js';

export type ColorScheme = 'dark' | 'light';

const STORAGE_KEY = 'kamc_theme';

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

  createEffect(() => {
    const scheme = colorScheme();
    document.documentElement.setAttribute('data-theme', `kamc-${scheme}`);
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
