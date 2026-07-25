import { useCallback, useEffect, useState, type ReactNode } from 'react';
import TutkaApp from './modes/tutka/TutkaApp';
import SalamaApp from './modes/salama/SalamaApp';
import HavainnotApp from './modes/havainnot/HavainnotApp';
import { VersionBadge } from './shared/components/VersionBadge';

export type ModeId = 'tutka' | 'salama' | 'havainnot';
export type Theme = 'dark' | 'light';

// The weather modes of the Tutka app. Adding one means a new
// src/modes/<mode>/ directory and an entry here.
const MODES: { id: ModeId; label: string; enabled: boolean }[] = [
  { id: 'tutka', label: 'Tutka', enabled: true },
  { id: 'salama', label: 'Salama', enabled: true },
  { id: 'havainnot', label: 'Havainnot', enabled: true },
];

// Per-mode glyphs for the mobile tab bar. Hidden on desktop (the switcher stays
// a text pill there), shown stacked above the label on phones. Stroked, 24-grid,
// currentColor so they inherit the active/idle tab colour.
const MODE_ICONS: Record<ModeId, ReactNode> = {
  // Radar: sweep arcs from a corner origin.
  tutka: (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M12 20a8 8 0 0 1 8-8M12 20a13 13 0 0 1 8-12" />
      <path d="M12 20h.01M4 20h4" />
      <circle cx="12" cy="20" r="1.4" />
    </svg>
  ),
  // Lightning: a bolt.
  salama: (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M13 2 5 14h5l-1 8 8-12h-5l1-8Z" />
    </svg>
  ),
  // Observations: a thermometer beside a gauge tick.
  havainnot: (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M10 14V5a2 2 0 1 1 4 0v9a4 4 0 1 1-4 0Z" />
      <path d="M17 7h3M17 11h3" />
    </svg>
  ),
};

function App() {
  const [mode, setMode] = useState<ModeId>(() => {
    const saved = localStorage.getItem('tutka-mode') as ModeId | null;
    return MODES.find((m) => m.id === saved && m.enabled) ? (saved as ModeId) : 'tutka';
  });

  useEffect(() => {
    localStorage.setItem('tutka-mode', mode);
  }, [mode]);

  // Theme is shell-owned so every mode shares one toggle and one preference.
  // The CSS tokens and each mode's basemap key off the data-theme attribute.
  const [theme, setTheme] = useState<Theme>(() => {
    const saved = localStorage.getItem('tutka-theme');
    return saved === 'light' ? 'light' : 'dark';
  });

  useEffect(() => {
    localStorage.setItem('tutka-theme', theme);
    document.documentElement.setAttribute('data-theme', theme);
  }, [theme]);

  const toggleTheme = useCallback(
    () => setTheme((t) => (t === 'dark' ? 'light' : 'dark')),
    []
  );

  return (
    <>
      {/* Desktop: a top-left text pill. Mobile: a fixed bottom tab bar in the
          thumb zone (see index.css). data-mode drives the active tab's accent to
          the current mode's colour, since this nav sits outside the mode roots. */}
      <nav className="mode-switcher" aria-label="Säämuoto" data-mode={mode}>
        {MODES.map((m) => (
          <button
            key={m.id}
            className={`mode-switcher__btn${mode === m.id ? ' mode-switcher__btn--active' : ''}`}
            disabled={!m.enabled}
            title={m.enabled ? m.label : `${m.label} — tulossa`}
            aria-current={mode === m.id ? 'page' : undefined}
            onClick={() => setMode(m.id)}
          >
            <span className="mode-switcher__icon">{MODE_ICONS[m.id]}</span>
            <span className="mode-switcher__label">{m.label}</span>
          </button>
        ))}
      </nav>

      {/* One prop signature for every mode — the sibling app's meri/raide
          inconsistency is not worth inheriting. */}
      {mode === 'tutka' && <TutkaApp theme={theme} onToggleTheme={toggleTheme} />}
      {mode === 'salama' && <SalamaApp theme={theme} onToggleTheme={toggleTheme} />}
      {mode === 'havainnot' && <HavainnotApp theme={theme} onToggleTheme={toggleTheme} />}

      <VersionBadge />
    </>
  );
}

export default App;
