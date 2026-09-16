'use strict';
(() => {
  let locale = 'zh-TW', theme = matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  try {
    const savedLocale = localStorage.getItem('flowledger:locale');
    const savedTheme = localStorage.getItem('flowledger:theme');
    if (['zh-TW', 'en', 'zh-CN'].includes(savedLocale)) locale = savedLocale;
    if (['light', 'dark'].includes(savedTheme)) theme = savedTheme;
  } catch { /* Preferences still work when browser storage is unavailable. */ }
  document.documentElement.lang = locale;
  document.documentElement.dataset.theme = theme;
})();
