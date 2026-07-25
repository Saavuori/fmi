import React, { useEffect, type ReactNode } from 'react';
import { SlidersHorizontal } from 'lucide-react';
import { useCollapsiblePanel } from '../hooks/useCollapsiblePanel';
import './Panel.css';

interface PanelProps {
  /** 'filter' → left rail / launcher-opened overlay; 'detail' → right rail / selection overlay. */
  variant: 'filter' | 'detail';
  isMobile: boolean;
  /**
   * Whether the panel exists at all. A mode passes `open={false}` for its filter
   * panel while a detail overlay is up on mobile, so the filter launcher isn't
   * stranded underneath it.
   */
  open: boolean;
  ariaLabel: string;
  /** Extra classes for the panel root (e.g. 'webcam-popup'). */
  className?: string;
  /** App-level collapsed flag. */
  collapsed: boolean;
  /** Toggles `collapsed` — the desktop sliver, the mobile launcher and the header button. */
  onToggleCollapse: () => void;
  /**
   * Panel header + body. Components gate their own body on
   * `!isMobile && isCollapsed`, so on mobile the whole panel is always rendered
   * when the overlay is up.
   */
  children: ReactNode;
}

const DESKTOP_CLASS: Record<PanelProps['variant'], string> = {
  filter: 'filter-panel',
  detail: 'detail-popup',
};

/**
 * Shared panel shell.
 *
 * Desktop is the existing `.glass-panel` rail with its collapse-sliver semantics
 * (`useCollapsiblePanel`). On phones a rail has nowhere to go, so the panel
 * becomes a plain full-screen overlay above the tab bar: it is either up or it
 * is not. That replaced a draggable bottom sheet with peek/half/full snap
 * points, which permanently occupied the bottom of the screen and squeezed the
 * radar timeline — the one control a phone visitor actually drags — into a
 * fraction of the width. Nothing here is anchored to the bottom edge any more,
 * so the timeline can span it whole.
 *
 * A filter panel is opened from a launcher button this component renders in the
 * collapsed state (there is no other way back to it). A detail panel is opened
 * by a map selection and closed from its own header, so it ignores `collapsed`,
 * which on mobile would only be able to hide it irrecoverably.
 */
export const Panel: React.FC<PanelProps> = ({
  variant,
  isMobile,
  open,
  ariaLabel,
  className,
  collapsed,
  onToggleCollapse,
  children,
}) => {
  // Desktop collapse props (spread onto the rail root; empty when expanded).
  const { className: collapsedClass, ...collapsibleProps } = useCollapsiblePanel(
    collapsed,
    onToggleCollapse,
    ariaLabel
  );

  // A viewport under the mobile breakpoint is usually a phone, but it can also be
  // a narrow desktop window — where the overlay is a dialog and Escape should
  // close it. Only the filter variant has something to close to.
  const escapable = isMobile && open && variant === 'filter' && !collapsed;
  useEffect(() => {
    if (!escapable) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onToggleCollapse();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [escapable, onToggleCollapse]);

  if (!isMobile) {
    return (
      <div
        className={`glass-panel ${DESKTOP_CLASS[variant]} ${className ?? ''} ${collapsedClass}`.trim()}
        {...collapsibleProps}
      >
        {children}
      </div>
    );
  }

  if (!open) return null;

  if (variant === 'filter' && collapsed) {
    return (
      <button
        className="panel-launcher"
        onClick={onToggleCollapse}
        aria-expanded={false}
        aria-label={ariaLabel}
        title={ariaLabel}
      >
        <SlidersHorizontal size={20} />
      </button>
    );
  }

  return (
    <div
      className={`mobile-panel mobile-panel--${variant} ${className ?? ''}`.trim()}
      role="dialog"
      aria-modal="true"
      aria-label={ariaLabel}
    >
      {children}
    </div>
  );
};
