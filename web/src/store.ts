import { create } from "zustand";
import { OttomanClient, type StatusResponse, type Layout, type Monitor, type AudioSink } from "./api";
import { sortedLayouts, sortedMonitors } from "./utils";

export const client = new OttomanClient({
  WITH_CREDENTIALS: true,
});

type AgentOnlineStatus = "online" | "offline" | "waking" | "shutting_down";

interface OttomanStore {
  // ── Auth ──────────────────────────────────────────────
  authed: boolean | null;
  checkAuth: () => Promise<void>;
  login: (token: string) => Promise<{ success: boolean; message?: string }>;
  logout: () => Promise<void>;

  // ── Server Status ─────────────────────────────────────
  status: StatusResponse | null;
  statusLoading: boolean;

  // ── Agent Status ──────────────────────────────────────
  agentStatus: AgentOnlineStatus;
  agentInfo: StatusResponse | null;
  agentLoading: boolean;

  // ── Layouts ───────────────────────────────────────────
  layouts: Layout[];
  currentLayout: string;
  layoutsLoading: boolean;
  layoutsError: string | null;
  switching: boolean;

  // ── Monitors ──────────────────────────────────────────
  monitors: Monitor[];
  monitorsLoading: boolean;
  monitorsError: string | null;

  // ── Audio ─────────────────────────────────────────────
  audioSinks: AudioSink[];
  audioLoading: boolean;
  audioError: string | null;

  // ── Refresh key (for WebSocket reconnect) ─────────────
  refreshKey: number;

  // ── Actions ───────────────────────────────────────────
  refreshAll: (silent: boolean) => Promise<void>;
  fetchStatus: (silent: boolean) => Promise<void>;
  fetchAgentStatus: (silent: boolean) => Promise<void>;
  fetchLayouts: (silent: boolean) => Promise<void>;
  fetchMonitors: (silent: boolean) => Promise<void>;
  fetchAudioSinks: (silent: boolean) => Promise<void>;

  // ── Audio Actions ─────────────────────────────────────
  setSinkVolume: (name: string, volume: number) => Promise<void>;
  setSinkMute: (name: string, muted: boolean) => Promise<void>;
  setSinkDefault: (name: string) => Promise<void>;

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
  _inflightAgentStatus: Promise<void> | null;
  _inflightLayouts: Promise<void> | null;
  _inflightMonitors: Promise<void> | null;
  _inflightAudio: Promise<void> | null;
  _prevAgentOnline: boolean | null;
}

export const useStore = create<OttomanStore>((set, get) => ({
  // ── Auth ──────────────────────────────────────────────
  authed: null,

  checkAuth: async () => {
    try {
      const res = await client.default.checkAuth();
      set({ authed: res.authenticated ?? false });
    } catch {
      set({ authed: false });
    }
  },

  login: async (token: string) => {
    const data = await client.default.auth({ token: token.trim() });
    if (data.success) {
      client.request.config.TOKEN = token.trim();
      set({ authed: true });
    }
    return data;
  },

  logout: async () => {
    await client.default.logout();
    client.request.config.TOKEN = undefined;
    get().stopPolling();
    set({
      authed: false,
      status: null,
      agentStatus: "offline",
      agentInfo: null,
      layouts: [],
      currentLayout: "",
      monitors: [],
    });
  },

  // ── Server Status ─────────────────────────────────────
  status: null,
  statusLoading: false,

  // ── Agent Status ──────────────────────────────────────
  agentStatus: "offline",
  agentInfo: null,
  agentLoading: false,

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

  // ── Audio ─────────────────────────────────────────────
  audioSinks: [],
  audioLoading: false,
  audioError: null,

  // ── Refresh key ───────────────────────────────────────
  refreshKey: 0,

  // ── Internal ──────────────────────────────────────────
  _pollTimer: null,
  _inflightStatus: null,
  _inflightAgentStatus: null,
  _inflightLayouts: null,
  _inflightMonitors: null,
  _inflightAudio: null,
  _prevAgentOnline: null,

  // ── Fetch Actions ─────────────────────────────────────

  fetchStatus: async (silent: boolean) => {
    if (get()._inflightStatus) return get()._inflightStatus!;

    if (!silent) set({ statusLoading: true });

    const promise = (async () => {
      try {
        const data = await client.default.getStatus();
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

  fetchAgentStatus: async (silent: boolean) => {
    if (get()._inflightAgentStatus) return get()._inflightAgentStatus!;

    if (!silent) set({ agentLoading: true });

    const promise = (async () => {
      try {
        const data = await client.default.getAgentStatus();
        const wasOnline = get()._prevAgentOnline;
        set({
          agentInfo: data,
          agentStatus: "online",
          _prevAgentOnline: true,
        });

        // Agent just came online — refresh layouts and monitors
        if (wasOnline === false) {
          get().fetchLayouts(true);
          get().fetchMonitors(true);
        }
      } catch {
        const { agentStatus } = get();
        // Preserve transitional states (waking/shutting_down), otherwise set offline
        if (agentStatus !== "waking" && agentStatus !== "shutting_down") {
          set({ agentStatus: "offline" });
        }
        set({ _prevAgentOnline: false });
      } finally {
        set({ agentLoading: false, _inflightAgentStatus: null });
      }
    })();

    set({ _inflightAgentStatus: promise });
    return promise;
  },

  fetchLayouts: async (silent: boolean) => {
    if (get()._inflightLayouts) return get()._inflightLayouts!;

    if (!silent) set({ layoutsLoading: true });

    const promise = (async () => {
      try {
        const data = await client.default.getLayouts();
        set({
          layouts: sortedLayouts(data.layouts),
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
        const data = await client.default.getMonitors();
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

  fetchAudioSinks: async (silent: boolean) => {
    if (get()._inflightAudio) return get()._inflightAudio!;

    if (!silent) set({ audioLoading: true });

    const promise = (async () => {
      try {
        const data = await client.default.getAudioSinks();
        set({ audioSinks: data, audioError: null });
      } catch {
        set({ audioError: "Failed to load audio sinks" });
      } finally {
        set({ audioLoading: false, _inflightAudio: null });
      }
    })();

    set({ _inflightAudio: promise });
    return promise;
  },

  refreshAll: async (silent: boolean) => {
    set((s) => ({ refreshKey: s.refreshKey + 1 }));
    await Promise.allSettled([
      get().fetchStatus(silent),
      get().fetchAgentStatus(silent),
      get().fetchLayouts(silent),
      get().fetchMonitors(silent),
      get().fetchAudioSinks(silent),
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
      const data = await client.default.switchLayout({ layout: id });
      if (data.success) {
        set({ currentLayout: data.current_layout ?? "" });
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
      const data = await client.default.removeLayout({ layout: id });
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
      const data = await client.default.saveCurrentLayout({ name, emoji });
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
      const data = await client.default.wake();
      if (data.success) {
        set({ agentStatus: "waking" });
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
      const data = await client.default.shutdown();
      if (data.success) {
        set({ agentStatus: "shutting_down" });
      } else {
        alert("Failed: " + data.message);
      }
    } catch {
      alert("Failed to send shutdown command");
    }
  },

  // ── Audio Actions ─────────────────────────────────────

  setSinkVolume: async (name: string, volume: number) => {
    // Optimistic local update so the slider stays responsive; the next poll
    // reconciles with the true value.
    set((s) => ({
      audioSinks: s.audioSinks.map((k) => (k.name === name ? { ...k, volume } : k)),
    }));
    try {
      await client.default.setAudioVolume({ name, volume });
    } catch {
      get().fetchAudioSinks(true);
    }
  },

  setSinkMute: async (name: string, muted: boolean) => {
    set((s) => ({
      audioSinks: s.audioSinks.map((k) => (k.name === name ? { ...k, muted } : k)),
    }));
    try {
      await client.default.setAudioVolume({ name, muted });
    } catch {
      get().fetchAudioSinks(true);
    }
  },

  setSinkDefault: async (name: string) => {
    set((s) => ({
      audioSinks: s.audioSinks.map((k) => ({ ...k, default: k.name === name })),
    }));
    try {
      await client.default.setAudioVolume({ name, default: true });
      get().fetchAudioSinks(true);
    } catch {
      get().fetchAudioSinks(true);
    }
  },
}));
