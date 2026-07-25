import { useCallback, useEffect, useState } from 'react';
import WeatherMap from './WeatherMap';
import { VersionBadge } from './shared/components/VersionBadge';

export type Theme = 'dark' | 'light';

/**
 * The shell. It owns the theme and nothing else.
 *
 * It used to own a mode switcher too — Tutka, Salama and Havainnot as three
 * apps, each with its own map, reached from a bottom tab bar. They are layers on
 * one map now: the same place at the same moment answered three ways is one
 * question, not three, and the tab bar was costing 64px of every phone screen to
 * navigate between answers nobody wanted separately.
 */
function App() {
  // Shell-owned so the whole app shares one toggle and one preference. The CSS
  // tokens and the basemap key off the data-theme attribute.
  const [theme, setTheme] = useState<Theme>(() => {
    const saved = localStorage.getItem('tutka-theme');
    return saved === 'light' ? 'light' : 'dark';
  });

  useEffect(() => {
    localStorage.setItem('tutka-theme', theme);
    document.documentElement.setAttribute('data-theme', theme);
  }, [theme]);

  const toggleTheme = useCallback(
    () => setTheme(t => (t === 'dark' ? 'light' : 'dark')),
    []
  );

  return (
    <>
      <WeatherMap theme={theme} onToggleTheme={toggleTheme} />
      <VersionBadge />
    </>
  );
}

export default App;
