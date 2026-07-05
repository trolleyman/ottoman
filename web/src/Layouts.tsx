import { useState } from "react";
import { computeUniformScale } from "./utils";
import { LayoutCard } from "./LayoutCard";
import { useStore } from "./store";

export function Layouts() {
  const layouts = useStore((s) => s.layouts);
  const currentLayout = useStore((s) => s.currentLayout);
  const loading = useStore((s) => s.layoutsLoading);
  const error = useStore((s) => s.layoutsError);
  const switching = useStore((s) => s.switching);
  const switchLayout = useStore((s) => s.switchLayout);
  const removeLayout = useStore((s) => s.removeLayout);
  const saveCurrentLayout = useStore((s) => s.saveCurrentLayout);

  const [showSaveForm, setShowSaveForm] = useState(false);
  const [newLayoutName, setNewLayoutName] = useState("");
  const [newLayoutEmoji, setNewLayoutEmoji] = useState("");

  const handleSave = async () => {
    await saveCurrentLayout(newLayoutName, newLayoutEmoji);
    setShowSaveForm(false);
    setNewLayoutName("");
    setNewLayoutEmoji("");
  };

  return (
    <section>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-zinc-200 flex items-center gap-2">
          Layouts
          {loading && layouts.length > 0 && (
            <div className="w-3.5 h-3.5 border-2 border-zinc-600 border-t-zinc-400 rounded-full animate-spin" />
          )}
        </h2>
        <button
          onClick={() => setShowSaveForm(!showSaveForm)}
          className="w-7 h-7 flex items-center justify-center bg-zinc-800 hover:bg-zinc-700 text-zinc-300 rounded-md transition-colors border border-zinc-700 cursor-pointer text-lg leading-none"
        >
          {showSaveForm ? "\u00d7" : "+"}
        </button>
      </div>

      {showSaveForm && (
        <div className="mb-6 p-4 rounded-xl border border-zinc-700 bg-zinc-800/50 flex flex-col sm:flex-row gap-3 items-end sm:items-center">
          <div className="flex-1 w-full">
            <label className="block text-xs text-zinc-500 mb-1">Name</label>
            <input
              type="text"
              value={newLayoutName}
              onChange={(e) => setNewLayoutName(e.target.value)}
              className="w-full rounded-md border border-zinc-600 bg-zinc-900 px-3 py-1.5 text-sm text-zinc-100 focus:outline-none focus:border-blue-500"
              placeholder="My Layout"
            />
          </div>
          <div className="w-full sm:w-24">
            <label className="block text-xs text-zinc-500 mb-1">Emoji</label>
            <input
              type="text"
              value={newLayoutEmoji}
              onChange={(e) => setNewLayoutEmoji(e.target.value)}
              className="w-full rounded-md border border-zinc-600 bg-zinc-900 px-3 py-1.5 text-sm text-zinc-100 focus:outline-none focus:border-blue-500"
              placeholder="🖥️"
            />
          </div>
          <button
            onClick={() => void handleSave()}
            disabled={!newLayoutName.trim()}
            className="w-full sm:w-auto rounded-md bg-blue-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
          >
            Save
          </button>
        </div>
      )}

      {loading && layouts.length === 0 && !error ? (
        <div className="text-zinc-500 text-sm">Loading layouts...</div>
      ) : error ? (
        <div className="text-red-400 text-sm">{error}</div>
      ) : layouts.length === 0 ? (
        <div className="text-zinc-500 text-sm">No layouts found.</div>
      ) : (() => {
        const layoutScale = computeUniformScale(layouts, 500, 300);
        return (
          <div className="flex flex-wrap gap-3">
            {layouts.map((l) => (
              <LayoutCard
                key={l.id}
                layout={l}
                isCurrent={l.id === currentLayout}
                disabled={switching}
                scale={layoutScale}
                onClick={() => void switchLayout(l.id)}
                onDelete={() => void removeLayout(l.id)}
              />
            ))}
          </div>
        );
      })()}
    </section>
  );
}
