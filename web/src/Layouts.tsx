import { useState, useEffect, useCallback } from "react";
import type { Layout, LayoutsResponse, SwitchResponse } from "./types";
import { computeUniformScale, fetchJSON, sortedLayouts } from "./utils";
import { LayoutCard } from "./LayoutCard";

export function Layouts({
  authed,
  refreshKey,
  onChange,
}: {
  authed: boolean;
  refreshKey: number;
  onChange: () => void;
}) {
  const [layouts, setLayouts] = useState<Layout[]>([]);
  const [currentLayout, setCurrentLayout] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [switching, setSwitching] = useState(false);

  const [showSaveForm, setShowSaveForm] = useState(false);
  const [newLayoutName, setNewLayoutName] = useState("");
  const [newLayoutEmoji, setNewLayoutEmoji] = useState("");

  const fetchLayouts = useCallback(async () => {
    if (!authed) return;
    setLoading(true);
    try {
      const layoutsData = await fetchJSON<LayoutsResponse>("/api/layouts");
      setLayouts(sortedLayouts(layoutsData.layouts ?? []));
      setCurrentLayout(layoutsData.current_layout ?? "");
      setError(null);
    } catch (e) {
      setError("Failed to load layouts");
    } finally {
      setLoading(false);
    }
  }, [authed]);

  useEffect(() => {
    fetchLayouts();
  }, [fetchLayouts, refreshKey]);

  const switchLayout = async (name: string) => {
    if (switching || name === currentLayout) return;
    setSwitching(true);
    try {
      const res = await fetch("/api/layouts/switch", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ layout: name }),
      });
      const data: SwitchResponse = await res.json();
      if (data.success) {
        setCurrentLayout(data.current_layout);
        onChange();
      } else {
        alert(data.message || "Switch failed");
      }
    } catch (e) {
      alert(e instanceof Error ? e.message : "Switch failed");
    } finally {
      setSwitching(false);
    }
  };

  const removeLayout = async (id: string) => {
    if (!confirm("Are you sure you want to delete this layout?")) return;
    try {
      const res = await fetch("/api/layouts/remove", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ layout: id }),
      });
      const data = await res.json();
      if (data.success) {
        fetchLayouts();
      } else {
        alert(data.message || "Failed to remove layout");
      }
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed to remove layout");
    }
  };

  const handleSave = async () => {
    try {
      const res = await fetch("/api/layouts/save-current", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: newLayoutName, emoji: newLayoutEmoji }),
      });
      const data = await res.json();
      if (data.success) {
        fetchLayouts();
      } else {
        alert(data.message || "Failed to save layout");
      }
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed to save layout");
    }

    setShowSaveForm(false);
    setNewLayoutName("");
    setNewLayoutEmoji("");
  };

  return (
    <section>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-zinc-200">Layouts</h2>
        {layouts.length > 0 && (
          <button
            onClick={() => setShowSaveForm(!showSaveForm)}
            className="text-xs bg-zinc-800 hover:bg-zinc-700 text-zinc-300 px-3 py-1.5 rounded-md transition-colors border border-zinc-700 cursor-pointer"
          >
            {showSaveForm ? "Cancel" : "Save Current"}
          </button>
        )}
      </div>

      {loading && layouts.length === 0 && !error ? (
        <div className="text-zinc-500 text-sm">Loading layouts...</div>
      ) : error ? (
        <div className="text-red-400 text-sm">{error}</div>
      ) : layouts.length === 0 ? (
        <div className="text-zinc-500 text-sm">No layouts found.</div>
      ) : (() => {
        const layoutScale = computeUniformScale(layouts, 500, 300);
        return (
          <>
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
                  onClick={handleSave}
                  disabled={!newLayoutName.trim()}
                  className="w-full sm:w-auto rounded-md bg-blue-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
                >
                  Save
                </button>
              </div>
            )}

            <div className="flex flex-wrap gap-3">
              {layouts.map((l) => (
                <LayoutCard
                  key={l.id}
                  layout={l}
                  isCurrent={l.id === currentLayout}
                  disabled={switching}
                  scale={layoutScale}
                  onClick={() => switchLayout(l.id)}
                  onDelete={() => removeLayout(l.id)}
                />
              ))}
            </div>
          </>
        );
      })()}
    </section>
  );
}
