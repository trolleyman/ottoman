import { useState } from "react";
import { Plus, X } from "lucide-react";
import { computeUniformScale } from "./utils";
import { LayoutCard } from "./LayoutCard";
import { Section, SectionState } from "./Section";
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
  const updateLayout = useStore((s) => s.updateLayout);

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
    <Section
      title="Layouts"
      loading={loading && layouts.length > 0}
      actions={
        <button
          onClick={() => setShowSaveForm(!showSaveForm)}
          title={showSaveForm ? "Cancel" : "Save current layout"}
          aria-label={showSaveForm ? "Cancel" : "Save current layout"}
          className="w-7 h-7 flex items-center justify-center bg-zinc-800 hover:bg-zinc-700 text-zinc-300 rounded-md transition-colors border border-zinc-700 cursor-pointer"
        >
          {showSaveForm ? <X className="w-4 h-4" /> : <Plus className="w-4 h-4" />}
        </button>
      }
    >
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
        <SectionState>Loading layouts…</SectionState>
      ) : error ? (
        <SectionState>{error}</SectionState>
      ) : layouts.length === 0 ? (
        <SectionState>No layouts saved yet.</SectionState>
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
                onUpdate={(changes) => updateLayout(l.id, changes)}
              />
            ))}
          </div>
        );
      })()}
    </Section>
  );
}
