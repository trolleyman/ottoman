import { useState } from "react";
import { Check, Pencil, Trash2 } from "lucide-react";
import { sortedLayoutMonitors } from "./utils";
import { MiniLayoutPreview } from "./MiniLayoutPreview";
import type { Layout } from "./api";

// LayoutEditor is the inline form shown when a card is put into edit mode. It
// edits the three user-facing bits of layout metadata — emoji, name and aliases
// — leaving the captured monitor geometry untouched.
function LayoutEditor({
  layout,
  onSave,
  onCancel,
}: {
  layout: Layout;
  onSave: (changes: { name: string; emoji: string; aliases: string[] }) => Promise<boolean>;
  onCancel: () => void;
}) {
  const [name, setName] = useState(layout.name);
  const [emoji, setEmoji] = useState(layout.emoji ?? "");
  // Aliases are edited as a comma/space separated string for simplicity.
  const [aliases, setAliases] = useState((layout.aliases ?? []).join(", "));
  const [saving, setSaving] = useState(false);

  const submit = async () => {
    if (!name.trim() || saving) return;
    setSaving(true);
    const ok = await onSave({
      name: name.trim(),
      emoji: emoji.trim(),
      aliases: aliases
        .split(/[,\s]+/)
        .map((a) => a.trim())
        .filter(Boolean),
    });
    setSaving(false);
    if (ok) onCancel();
  };

  const field =
    "w-full rounded-md border border-zinc-600 bg-zinc-900 px-2.5 py-1.5 text-sm text-zinc-100 focus:outline-none focus:border-blue-500";

  return (
    <div className="relative overflow-hidden rounded-xl p-4 flex flex-col gap-3 w-full min-w-[180px] bg-gradient-to-br from-zinc-800/80 via-zinc-800/50 to-zinc-900/80 border border-zinc-600">
      <div className="flex gap-2">
        <input
          value={emoji}
          onChange={(e) => setEmoji(e.target.value)}
          className={`${field} w-14 text-center`}
          placeholder="🖥️"
          aria-label="Emoji"
        />
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && submit()}
          className={field}
          placeholder="Layout name"
          aria-label="Name"
          autoFocus
        />
      </div>
      <label className="flex flex-col gap-1">
        <span className="text-[10px] uppercase tracking-wide text-zinc-500">
          Aliases
        </span>
        <input
          value={aliases}
          onChange={(e) => setAliases(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && submit()}
          className={`${field} font-mono`}
          placeholder="work, 1, gaming"
          aria-label="Aliases"
        />
        <span className="text-[10px] text-zinc-600">
          Shortcuts to switch to this layout — separate with commas.
        </span>
      </label>
      <div className="flex gap-2">
        <button
          onClick={submit}
          disabled={!name.trim() || saving}
          className="flex-1 flex items-center justify-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
        >
          <Check className="w-4 h-4" />
          {saving ? "Saving…" : "Save"}
        </button>
        <button
          onClick={onCancel}
          className="rounded-md bg-zinc-700/60 hover:bg-zinc-600/60 px-3 py-1.5 text-sm text-zinc-200 cursor-pointer"
        >
          Cancel
        </button>
      </div>
    </div>
  );
}

export function LayoutCard({
  layout,
  isCurrent,
  disabled,
  scale,
  onClick,
  onDelete,
  onUpdate,
}: {
  layout: Layout;
  isCurrent: boolean;
  disabled: boolean;
  scale: number;
  onClick: () => void;
  onDelete?: () => void;
  onUpdate?: (changes: { name: string; emoji: string; aliases: string[] }) => Promise<boolean>;
}) {
  const [editing, setEditing] = useState(false);
  const enabled = sortedLayoutMonitors(layout.monitors ?? []);
  const aliasList = layout.aliases ?? [];

  // Cap the card width (max-w-xs) so a small number of layouts don't stretch to
  // fill the whole row; they still grow to share space evenly up to that cap.
  const wrapperClass = "relative group mb-auto flex-grow basis-56 max-w-xs";

  if (editing && onUpdate) {
    return (
      <div className={wrapperClass}>
        <LayoutEditor
          layout={layout}
          onSave={onUpdate}
          onCancel={() => setEditing(false)}
        />
      </div>
    );
  }

  return (
    <div className={wrapperClass}>
      {/* Hover toolbar: edit + delete */}
      <div className="absolute -top-2 -right-2 flex gap-1 opacity-0 group-hover:opacity-100 transition-all z-20">
        {onUpdate && (
          <button
            onClick={(e) => {
              e.stopPropagation();
              setEditing(true);
            }}
            className="w-6 h-6 rounded-full bg-zinc-800 border border-zinc-600 text-zinc-400 hover:text-blue-400 hover:border-blue-400 flex items-center justify-center shadow-lg cursor-pointer"
            title="Edit layout"
            aria-label="Edit layout"
          >
            <Pencil className="w-3 h-3" />
          </button>
        )}
        {onDelete && (
          <button
            onClick={(e) => {
              e.stopPropagation();
              onDelete();
            }}
            className="w-6 h-6 rounded-full bg-zinc-800 border border-zinc-600 text-zinc-400 hover:text-red-400 hover:border-red-400 flex items-center justify-center shadow-lg cursor-pointer"
            title="Delete layout"
            aria-label="Delete layout"
          >
            <Trash2 className="w-3 h-3" />
          </button>
        )}
      </div>
      <button
        onClick={onClick}
        disabled={disabled}
        className={`
          relative overflow-hidden rounded-xl text-sm font-medium transition-all cursor-pointer p-4 flex flex-col w-full gap-1.5 min-w-[180px] text-left
          ${isCurrent
            ? "bg-gradient-to-br from-blue-500/20 via-blue-500/10 to-indigo-500/20 text-blue-400 border border-blue-500/40 ring-1 ring-blue-500/20"
            : "bg-gradient-to-br from-zinc-800/80 via-zinc-800/50 to-zinc-900/80 text-zinc-300 border border-zinc-700/50 hover:border-zinc-600 hover:from-zinc-800 hover:to-zinc-800/80"
          }
          disabled:opacity-50 disabled:cursor-wait
        `}
      >
        {/* Header row: name left, emoji right */}
        <span className="flex items-start justify-between gap-3 w-full">
          <span className="flex flex-col gap-1 min-w-0">
            <span className="flex items-center gap-2">
              {isCurrent && (
                <span className="inline-block w-2 h-2 rounded-full bg-blue-400 shrink-0" />
              )}
              <span className="font-semibold truncate">{layout.name}</span>
            </span>
            {aliasList.length > 0 && (
              <span className="flex flex-wrap gap-1">
                {aliasList.map((a) => (
                  <span
                    key={a}
                    className="text-[10px] font-mono text-zinc-400 bg-zinc-900/60 border border-zinc-700/60 rounded px-1.5 py-0.5 leading-none"
                  >
                    {a}
                  </span>
                ))}
              </span>
            )}
          </span>
          {layout.emoji && <span className="text-lg leading-none">{layout.emoji}</span>}
        </span>

        {/* Monitor list */}
        {enabled.length > 0 && (
          <div className="flex flex-col gap-1 mt-2 w-full">
            {enabled.map((m, i) => (
              <div
                key={m.port || m.edid || i}
                className={`grid grid-cols-[auto_1fr_auto] items-center gap-2 text-[11px] px-2 py-1.5 rounded border ${m.primary
                  ? "bg-blue-500/10 border-blue-500/20"
                  : "bg-zinc-900/40 border-transparent"
                  }`}
              >
                <span className="truncate text-zinc-300 font-medium" title={m.name || m.port}>
                  {m.name || m.port || "Unknown"}
                </span>
                <span className="font-mono text-zinc-500">{m.width}x{m.height}</span>
                <span className="font-mono text-zinc-600 text-[10px]">{m.edid}</span>
              </div>
            ))}
          </div>
        )}
      </button>

      {/* Hover preview — below card with notch */}
      {enabled.length > 0 && (
        <div className="hidden sm:block absolute left-1/2 -translate-x-1/2 top-full mt-2 z-10 opacity-0 group-hover:opacity-100 pointer-events-none group-hover:pointer-events-auto transition-opacity duration-150">
          {/* Notch */}
          <div className="mx-auto w-0 h-0 border-l-[6px] border-l-transparent border-r-[6px] border-r-transparent border-b-[6px] border-b-zinc-700" />
          <div className="rounded-xl border border-zinc-700 bg-gradient-to-b from-zinc-900 to-zinc-950 p-3 shadow-2xl">
            <MiniLayoutPreview monitors={layout.monitors} scale={scale} />
          </div>
        </div>
      )}
    </div>
  );
}
