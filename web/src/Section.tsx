import type { ReactNode } from "react";

// Section is the shared frame for the dashboard's top-level groups (Layouts,
// Monitors, Audio, …). It keeps the header — title, a refresh spinner, optional
// right-side meta text and action buttons — visually identical everywhere so the
// sections read as one system.
export function Section({
  title,
  loading = false,
  meta,
  actions,
  children,
}: {
  title: string;
  /** Show the small header spinner (use for background refreshes with data already on screen). */
  loading?: boolean;
  /** Muted right-aligned info, e.g. "2 active / 3 total". */
  meta?: ReactNode;
  /** Right-aligned controls, e.g. an add button. */
  actions?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section>
      <div className="flex items-center justify-between gap-3 mb-4">
        <h2 className="text-lg font-semibold text-zinc-200 flex items-center gap-2">
          {title}
          {loading && (
            <span className="w-3.5 h-3.5 border-2 border-zinc-600 border-t-zinc-400 rounded-full animate-spin" />
          )}
        </h2>
        {(meta || actions) && (
          <div className="flex items-center gap-3">
            {meta && <span className="text-xs text-zinc-500">{meta}</span>}
            {actions}
          </div>
        )}
      </div>
      {children}
    </section>
  );
}

// SectionState is the consistent placeholder used inside a Section's body while
// it loads, errors, or has nothing to show. It deliberately uses muted (never
// red) text — a section failing to load is almost always just the desktop being
// offline, which is surfaced prominently elsewhere, so it shouldn't read as an
// alarm here.
export function SectionState({ children }: { children: ReactNode }) {
  return <div className="text-zinc-500 text-sm">{children}</div>;
}
