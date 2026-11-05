/**
 * ThemeProvider — light | dark | system; preferensi disimpan lokal (localStorage).
 */
import React, { createContext, useContext, useEffect, useState, useCallback } from 'react';

export type ThemeMode = 'light' | 'dark' | 'system';

export interface ThemeCtx {
  mode: ThemeMode;
  isDark: boolean;
  setMode: (m: ThemeMode) => void;
}

const Ctx = createContext<ThemeCtx>({ mode: 'system', isDark: false, setMode: () => {} });

function apply(mode: ThemeMode): boolean {
  const sysDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
  const dark = mode === 'dark' || (mode === 'system' && sysDark);
  document.documentElement.classList.toggle('dark', dark);
  return dark;
}

export const ThemeProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [mode, setModeState] = useState<ThemeMode>(() => {
    const stored = window.localStorage.getItem('simarc.theme');
    return stored === 'light' || stored === 'dark' || stored === 'system' ? stored : 'system';
  });
  const [isDark, setIsDark] = useState(() => apply(mode));

  useEffect(() => {
    document.documentElement.classList.add('theme-watch');
    const md = window.matchMedia('(prefers-color-scheme: dark)');
    const handler = () => { if (mode === 'system') setIsDark(apply('system')); };
    md.addEventListener?.('change', handler);
    return () => md.removeEventListener?.('change', handler);
  }, [mode]);

  const setMode = useCallback((m: ThemeMode) => {
    setModeState(m);
    window.localStorage.setItem('simarc.theme', m);
    setIsDark(apply(m));
  }, []);

  return <Ctx.Provider value={{ mode, isDark, setMode }}>{children}</Ctx.Provider>;
};

export const useTheme = () => useContext(Ctx);
