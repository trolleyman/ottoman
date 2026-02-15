import { create } from "zustand";
import type {
  StatusResponse,
  ClientStatusResponse,
  Layout,
  MonitorInfo,
  LayoutsResponse,
  SwitchResponse,
} from "./types";
import { fetchJSON, sortedLayouts, sortedMonitors } from "./utils";

type ClientOnlineStatus = "online" | "offline" | "waking" | "shutting_down";

interface OttomanStore {
  // ── Auth ──────────────────────────────────────────────
  authed: boolean | null;
  checkAuth: () => Promise<void>;
  login: (token: string) => Promise<{ success: boolean; message?: string }>;
  logout: () => Promise<void>;

  // ── Server Status ─────────────────────────────────────
  status: StatusResponse | null;
  statusLoading: boolean;

  // ── Client Status ─────────────────────────────────────
  clientStatus: ClientOnlineStatus;
  clientInfo: ClientStatusResponse | null;
  clientLoading: boolean;

  // ── Layouts ───────────────────────────────────────────
  layouts: Layout[];
  currentLayout: string;
  layoutsLoading: boolean;
  layoutsError: string | null;
  switching: boolean;

  // ── Monitors ──────────────────────────────────────────
  monitors: MonitorInfo[];
  monitorsLoading: boolean;
  monitorsError: string | null;

  // ── Refresh key (for WebSocket reconnect) ─────────────
  refreshKey: number;

  // ── Actions ───────────────────────────────────────────
  refreshAll: (silent: boolean) => Promise<void>;
  fetchStatus: (silent: boolean) => Promise<void>;
  fetchClientStatus: (silent: boolean) => Promise<void>;
  fetchLayouts: (silent: boolean) => Promise<void>;
  fetchMonitors: (silent: boolean) => Promise<void>;

  // ── Layout Actions ────────────────────────────────────
  switchLayout: (id: string) => Promise<void>;
  removeLayout: (id: string) => Promise<void>;
  saveCurrentLayout: (name: string, emoji: string) => Promise<void>;

  // ── Power Actions ─────────────────────────────────────
  wake: () => Promise<void>;
  shutdown: () => Promise<void>;

  // ── Polling Control ───────────────────────────────────
  startPolling: () => void;
  stopPolling: () => void;

  // ── Internal ──────────────────────────────────────────
  _pollTimer: ReturnType<typeof setInterval> | null;
  _inflightStatus: Promise<void> | null;
  _inflightClientStatus: Promise<void> | null;
  _inflightLayouts: Promise<void> | null;
  _inflightMonitors: Promise<void> | null;
  _prevClientOnline: boolean | null;
}

export const useStore = create<OttomanStore>((set, get) => ({
  // ── Auth ──────────────────────────────────────────────
  authed: null,

  checkAuth: async () => {
    try {
      const res = await fetch("/api/auth/check");
      set({ authed: res.ok });
    } catch {
      set({ authed: false });
    }
  },

  login: async (token: string) => {
    const res = await fetch("/api/auth", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token: token.trim() }),
    });
    const data = await res.json();
    if (data.success) {
      set({ authed: true });
    }
    return data;
  },

  logout: async () => {
    await fetch("/api/auth/logout", { method: "POST" });
    get().stopPolling();
    set({
      authed: false,
      status: null,
      clientStatus: "offline",
      clientInfo: null,
      layouts: [],
      currentLayout: "",
      monitors: [],
    });
  },

  // ── Server Status ─────────────────────────────────────
  status: null,
  statusLoading: false,

  // ── Client Status ─────────────────────────────────────
  clientStatus: "offline",
  clientInfo: null,
  clientLoading: false,

  // ── Layouts ───────────────────────────────────────────
  layouts: [],
  currentLayout: "",
  layoutsLoading: false,
  layoutsError: null,
  switching: false,

  // ── Monitors ──────────────────────────────────────────
  monitors: [],
  monitorsLoading: false,
  monitorsError: null,

  // ── Refresh key ───────────────────────────────────────
  refreshKey: 0,

  // ── Internal ──────────────────────────────────────────
  _pollTimer: null,
  _inflightStatus: null,
  _inflightClientStatus: null,
  _inflightLayouts: null,
  _inflightMonitors: null,
  _prevClientOnline: null,

  // ── Fetch Actions ─────────────────────────────────────

  fetchStatus: async (silent: boolean) => {
    if (get()._inflightStatus) return get()._inflightStatus!;

    if (!silent) set({ statusLoading: true });

    const promise = (async () => {
      try {
        const data = await fetchJSON<StatusResponse>("/api/status");
        set({ status: data });
      } catch {
        // Ignore errors
      } finally {
        set({ statusLoading: false, _inflightStatus: null });
      }
    })();

    set({ _inflightStatus: promise });
    return promise;
  },

  fetchClientStatus: async (silent: boolean) => {
    if (get()._inflightClientStatus) return get()._inflightClientStatus!;

    if (!silent) set({ clientLoading: true });

    const promise = (async () => {
      try {
        const data = await fetchJSON<ClientStatusResponse>("/api/status/client");
        const wasOnline = get()._prevClientOnline;
        set({
          clientInfo: data,
          clientStatus: "online",
          _prevClientOnline: true,
        });

        // Client just came online — refresh layouts and monitors
        if (wasOnline === false) {
          get().fetchLayouts(true);
          get().fetchMonitors(true);
        }
      } catch {
        const { clientStatus } = get();
        // Preserve transitional states (waking/shutting_down), otherwise set offline
        if (clientStatus !== "waking" && clientStatus !== "shutting_down") {
          set({ clientStatus: "offline" });
        }
        set({ _prevClientOnline: false });
      } finally {
        set({ clientLoading: false, _inflightClientStatus: null });
      }
    })();

    set({ _inflightClientStatus: promise });
    return promise;
  },

  fetchLayouts: async (silent: boolean) => {
    if (get()._inflightLayouts) return get()._inflightLayouts!;

    if (!silent) set({ layoutsLoading: true });

    const promise = (async () => {
      try {
        const data = await fetchJSON<LayoutsResponse>("/api/layouts");
        set({
          layouts: sortedLayouts(data.layouts ?? []),
          currentLayout: data.current_layout ?? "",
          layoutsError: null,
        });
      } catch {
        set({ layoutsError: "Failed to load layouts" });
      } finally {
        set({ layoutsLoading: false, _inflightLayouts: null });
      }
    })();

    set({ _inflightLayouts: promise });
    return promise;
  },

  fetchMonitors: async (silent: boolean) => {
    if (get()._inflightMonitors) return get()._inflightMonitors!;

    if (!silent) set({ monitorsLoading: true });

    const promise = (async () => {
      try {
        const data = await fetchJSON<MonitorInfo[]>("/api/monitors");
        set({
          monitors: sortedMonitors(data),
          monitorsError: null,
        });
      } catch {
        set({ monitorsError: "Failed to load monitors" });
      } finally {
        set({ monitorsLoading: false, _inflightMonitors: null });
      }
    })();

    set({ _inflightMonitors: promise });
    return promise;
  },

  refreshAll: async (silent: boolean) => {
    set((s) => ({ refreshKey: s.refreshKey + 1 }));
    await Promise.allSettled([
      get().fetchStatus(silent),
      get().fetchClientStatus(silent),
      get().fetchLayouts(silent),
      get().fetchMonitors(silent),
    ]);
  },

  // ── Polling ───────────────────────────────────────────

  startPolling: () => {
    if (get()._pollTimer) return;
    const timer = setInterval(() => {
      get().refreshAll(true);
    }, 3000);
    set({ _pollTimer: timer });
  },

  stopPolling: () => {
    const timer = get()._pollTimer;
    if (timer) {
      clearInterval(timer);
      set({ _pollTimer: null });
    }
  },

  // ── Layout Actions ────────────────────────────────────

  switchLayout: async (id: string) => {
    if (get().switching || id === get().currentLayout) return;
    set({ switching: true });
    try {
      const res = await fetch("/api/layouts/switch", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ layout: id }),
      });
      const data: SwitchResponse = await res.json();
      if (data.success) {
        set({ currentLayout: data.current_layout });
        get().refreshAll(false);
      } else {
        alert(data.message || "Switch failed");
      }
    } catch (e) {
      alert(e instanceof Error ? e.message : "Switch failed");
    } finally {
      set({ switching: false });
    }
  },

  removeLayout: async (id: string) => {
    if (!confirm("Are you sure you want to delete this layout?")) return;
    try {
      const res = await fetch("/api/layouts/remove", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ layout: id }),
      });
      const data = await res.json();
      if (data.success) {
        get().fetchLayouts(false);
      } else {
        alert(data.message || "Failed to remove layout");
      }
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed to remove layout");
    }
  },

  saveCurrentLayout: async (name: string, emoji: string) => {
    try {
      const res = await fetch("/api/layouts/save-current", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, emoji }),
      });
      const data = await res.json();
      if (data.success) {
        get().fetchLayouts(false);
      } else {
        alert(data.message || "Failed to save layout");
      }
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed to save layout");
    }
  },

  // ── Power Actions ─────────────────────────────────────

  wake: async () => {
    try {
      const res = await fetch("/api/wake", { method: "POST" });
      const data = await res.json();
      if (data.success) {
        set({ clientStatus: "waking" });
      } else {
        alert("Failed: " + data.message);
      }
    } catch {
      alert("Failed to send wake packet");
    }
  },

  shutdown: async () => {
    if (!confirm("Are you sure you want to shut down?")) return;
    try {
      const res = await fetch("/api/shutdown", { method: "POST" });
      const data = await res.json();
      if (data.success) {
        set({ clientStatus: "shutting_down" });
      } else {
        alert("Failed: " + data.message);
      }
    } catch {
      alert("Failed to send shutdown command");
    }
  },
}));
