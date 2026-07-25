import React, { useCallback, useEffect, useRef, useState } from 'react';
import { MapPin, Search, X } from 'lucide-react';

export interface Place {
  name: string;
  /** Municipality or region, when the name alone does not identify the place. */
  detail: string;
  latitude: number;
  longitude: number;
  /** Suggested zoom for the kind of thing this is — a city is not a farm road. */
  zoom: number;
}

interface PlaceSearchResponse {
  results: Place[];
  attribution: string;
}

interface PlaceSearchProps {
  onPick: (place: Place) => void;
}

const MIN_QUERY = 2;

/**
 * Finds a place on the map by name.
 *
 * It searches on submit — Enter or the button — rather than on every keystroke.
 * The results come from OpenStreetMap's geocoder via the backend, whose usage
 * policy rules out per-keystroke autocomplete; the backend caches and paces the
 * requests, and asking only when the viewer has finished typing is the other
 * half of keeping inside it.
 *
 * Collapsed it is a button the size of the other map controls, in the corner the
 * mode switcher used to occupy.
 */
export const PlaceSearch: React.FC<PlaceSearchProps> = ({ onPick }) => {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<Place[] | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const rootRef = useRef<HTMLDivElement | null>(null);

  // Ignore a response that arrived after a newer search was started.
  const requestRef = useRef(0);

  const close = useCallback(() => {
    setOpen(false);
    setQuery('');
    setResults(null);
    setError(null);
  }, []);

  useEffect(() => {
    if (open) inputRef.current?.focus();
  }, [open]);

  // Escape closes, and so does a click anywhere else — the popover covers the
  // map, and dismissing it should not require finding the small button again.
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') close();
    };
    const onDown = (e: MouseEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) close();
    };
    window.addEventListener('keydown', onKey);
    window.addEventListener('mousedown', onDown);
    return () => {
      window.removeEventListener('keydown', onKey);
      window.removeEventListener('mousedown', onDown);
    };
  }, [open, close]);

  const search = useCallback(async () => {
    const q = query.trim();
    if (q.length < MIN_QUERY) return;
    const id = ++requestRef.current;
    setPending(true);
    setError(null);
    try {
      const res = await fetch(`/api/places?q=${encodeURIComponent(q)}`);
      if (res.status === 429) throw new Error('busy');
      if (!res.ok) throw new Error(String(res.status));
      const body = (await res.json()) as PlaceSearchResponse;
      if (id !== requestRef.current) return;
      setResults(body.results);
    } catch (err) {
      if (id !== requestRef.current) return;
      setResults(null);
      setError(
        err instanceof Error && err.message === 'busy'
          ? 'Haku on ruuhkainen, yritä hetken kuluttua.'
          : 'Paikkahaku ei onnistunut.'
      );
    } finally {
      if (id === requestRef.current) setPending(false);
    }
  }, [query]);

  if (!open) {
    return (
      <div className="place-search-overlay" ref={rootRef}>
        <button
          className="place-search-launcher"
          onClick={() => setOpen(true)}
          aria-label="Hae paikkaa"
          title="Hae paikkaa"
        >
          <Search size={18} />
        </button>
      </div>
    );
  }

  return (
    <div className="place-search-overlay place-search-overlay--open" ref={rootRef}>
      <form
        className="place-search"
        onSubmit={e => {
          e.preventDefault();
          search();
        }}
        role="search"
      >
        <button
          type="submit"
          className="place-search-clear place-search-icon"
          aria-label="Hae"
          disabled={query.trim().length < MIN_QUERY}
        >
          <Search size={15} />
        </button>
        <input
          ref={inputRef}
          className="place-search-input"
          value={query}
          onChange={e => setQuery(e.target.value)}
          placeholder="Hae paikkaa…"
          aria-label="Hae paikkaa"
          enterKeyHint="search"
        />
        <button type="button" className="place-search-clear" onClick={close} aria-label="Sulje haku">
          <X size={15} />
        </button>

        {(pending || error || results) && (
          <div className="place-search-results">
            {pending && <div className="place-search-note">Haetaan…</div>}
            {!pending && error && <div className="place-search-note">{error}</div>}
            {!pending && !error && results?.length === 0 && (
              <div className="place-search-note">Ei osumia Suomesta.</div>
            )}
            {!pending &&
              !error &&
              results?.map(place => (
                <button
                  type="button"
                  className="place-search-result"
                  key={`${place.latitude},${place.longitude},${place.name}`}
                  onClick={() => {
                    onPick(place);
                    close();
                  }}
                >
                  <MapPin size={13} className="place-search-icon" />
                  <span className="place-search-name">{place.name}</span>
                  <span className="place-search-detail">{place.detail}</span>
                </button>
              ))}
            {!pending && !error && !!results?.length && (
              <div className="place-search-credit">Paikat: OpenStreetMap</div>
            )}
          </div>
        )}
      </form>
    </div>
  );
};
