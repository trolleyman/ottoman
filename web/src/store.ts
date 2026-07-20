import { create } from "zustand";
import { OttomanClient, type StatusResponse, type Layout, type Monitor, type AudioSink, type MonitorSettingsRequest } from "./api";
import { sortedLayouts, sortedMonitors } from "./utils";

export const client = new OttomanClient({
  WITH_CREDENTIALS: true,
});

type AgentOnlineStatus = "online" | "offline" | "waking" | "shutting_down";

/** Turn the verified switch outcome into a user-facing notice. Only "applied"
 *  is silently fine; the others mean the screen may not match what was asked
 *  for, which the user needs to know rather than being told it worked. */
function noticeForOutcome(
  outcome: string | undefined,
  message: string | undefined,
): { kind: "ok" | "warn"; text: string } | null {
  switch (outcome) {
    case "applied":
      return null;
    case "already-active":
      return {
        kind: "warn",
        text: "Nothing changed — the display server already reported this layout as active. If the screen disagrees, its state has drifted.",
      };
    case "rolled-back":
      return {
        kind: "warn",
        text: "The layout was applied but the display server reverted it moments later. The displays may not support this configuration right now.",
      };
    case "mismatch":
      return {
        kind: "warn",
        text: "The switch was accepted but the display never matched the layout.",
      };
    case "unverified":
      return { kind: "ok", text: "Switched (result could not be verified)." };
    default:
      return message ? { kind: "ok", text: message } : null;
  }
}

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
  /** Result of the last layout switch, shown inline. The display server can
   *  accept a switch and then revert it, or report nothing changed, so the UI
   *  reports what actually happened rather than assuming success. */
  layoutNotice: { kind: "ok" | "warn"; text: string } | null;

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

  // ── Monitor Control Actions ───────────────────────────
  setMonitorBrightness: (edid: string, brightness: number) => Promise<void>;
  setMonitorPower: (edid: string, on: boolean) => Promise<void>;
  probeMonitorPower: (edid: string) => Promise<boolean>;
  saveMonitorSettings: (settings: MonitorSettingsRequest) => Promise<boolean>;
  setMonitorVolume: (edid: string, volume: number) => Promise<void>;
  setMonitorMute: (edid: string, muted: boolean) => Promise<void>;
  pairMonitor: (edid: string) => Promise<void>;

  // ── Layout Actions ────────────────────────────────────
  switchLayout: (id: string) => Promise<void>;
  dismissLayoutNotice: () => void;
  removeLayout: (id: string) => Promise<void>;
  saveCurrentLayout: (name: string, emoji: string) => Promise<void>;
  updateLayout: (
    id: string,
    changes: {
      name?: string;
      emoji?: string;
      aliases?: string[];
      /** Replace the layout's monitors with the current display setup. */
      captureMonitors?: boolean;
    },
  ) => Promise<boolean>;

  // ── Power Actions ─────────────────────────────────────
  wake: (target?: "linux" | "windows") => Promise<void>;
  shutdown: () => Promise<void>;
  reboot: (target: "linux" | "windows") => Promise<void>;

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
  layoutNotice: null,

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
          void get().fetchLayouts(true);
          void get().fetchMonitors(true);
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
        set({ layoutsError: "Layouts unavailable — the desktop may be offline." });
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
        set({ monitorsError: "Monitors unavailable — the desktop may be offline." });
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
      void get().refreshAll(true);
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

  // Re-applying the layout that's already selected is allowed on purpose: if the
  // display server's state has drifted from what's actually on screen, tapping
  // the current layout is how you force them back into sync.
  switchLayout: async (id: string) => {
    if (get().switching) return;
    set({ switching: true, layoutNotice: null });
    try {
      const data = await client.default.switchLayout({ layout: id });
      if (data.success) {
        set({ currentLayout: data.current_layout ?? "" });
        set({ layoutNotice: noticeForOutcome(data.outcome, data.message) });
        void get().refreshAll(false);
      } else {
        set({ layoutNotice: { kind: "warn", text: data.message || "Switch failed" } });
      }
    } catch (e) {
      set({
        layoutNotice: {
          kind: "warn",
          text: e instanceof Error ? e.message : "Switch failed",
        },
      });
    } finally {
      set({ switching: false });
    }
  },

  dismissLayoutNotice: () => set({ layoutNotice: null }),

  removeLayout: async (id: string) => {
    if (!confirm("Are you sure you want to delete this layout?")) return;
    try {
      const data = await client.default.removeLayout({ layout: id });
      if (data.success) {
        void get().fetchLayouts(false);
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
        void get().fetchLayouts(false);
      } else {
        alert(data.message || "Failed to save layout");
      }
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed to save layout");
    }
  },

  updateLayout: async (id, changes) => {
    // Optimistic local update so the edited card settles immediately; a
    // failure re-fetches to reconcile.
    set((s) => ({
      layouts: s.layouts.map((l) =>
        l.id === id
          ? {
              ...l,
              ...(changes.name !== undefined ? { name: changes.name } : {}),
              ...(changes.emoji !== undefined ? { emoji: changes.emoji } : {}),
              ...(changes.aliases !== undefined ? { aliases: changes.aliases } : {}),
            }
          : l,
      ),
    }));
    try {
      const { captureMonitors, ...meta } = changes;
      const data = await client.default.updateLayout({
        id,
        ...meta,
        ...(captureMonitors ? { capture_monitors: true } : {}),
      });
      if (data.success) {
        void get().fetchLayouts(true);
        return true;
      }
      alert(data.message || "Failed to update layout");
      void get().fetchLayouts(true);
      return false;
    } catch (e) {
      // The generated client throws ApiError with a generic message but keeps the
      // server's ErrorResponse in `.body` — surface that (e.g. an alias conflict).
      const body = (e as { body?: { error?: string } })?.body;
      alert(body?.error || (e instanceof Error ? e.message : "Failed to update layout"));
      void get().fetchLayouts(true);
      return false;
    }
  },

  // ── Power Actions ─────────────────────────────────────

  wake: async (target?: "linux" | "windows") => {
    try {
      const data = await client.default.wake(target ? { target } : undefined);
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

  reboot: async (target: "linux" | "windows") => {
    const label = target === "windows" ? "reboot into Windows" : "reboot";
    if (!confirm(`Are you sure you want to ${label}?`)) return;
    try {
      const data = await client.default.boot({ target });
      if (data.success) {
        set({ agentStatus: "shutting_down" });
      } else {
        alert("Failed: " + data.message);
      }
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed to send reboot command");
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
      void get().fetchAudioSinks(true);
    }
  },

  setSinkMute: async (name: string, muted: boolean) => {
    set((s) => ({
      audioSinks: s.audioSinks.map((k) => (k.name === name ? { ...k, muted } : k)),
    }));
    try {
      await client.default.setAudioVolume({ name, muted });
    } catch {
      void get().fetchAudioSinks(true);
    }
  },

  setSinkDefault: async (name: string) => {
    set((s) => ({
      audioSinks: s.audioSinks.map((k) => ({ ...k, default: k.name === name })),
    }));
    try {
      await client.default.setAudioVolume({ name, default: true });
      void get().fetchAudioSinks(true);
    } catch {
      void get().fetchAudioSinks(true);
    }
  },

  // ── Monitor Control Actions ───────────────────────────

  setMonitorBrightness: async (edid: string, brightness: number) => {
    // Optimistic update so the slider stays responsive.
    set((s) => ({
      monitors: s.monitors.map((m) => (m.edid === edid ? { ...m, brightness } : m)),
    }));
    try {
      await client.default.setMonitorBrightness({ edid, brightness });
    } catch {
      void get().fetchMonitors(true);
    }
  },

  setMonitorPower: async (edid: string, on: boolean) => {
    try {
      await client.default.setMonitorPower({ edid, on });
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed to set monitor power");
    }
  },

  // probeMonitorPower reports whether a monitor currently answers its control
  // backend (true ≈ powered on). Used to confirm a power toggle by polling.
  probeMonitorPower: async (edid: string) => {
    try {
      const res = await client.default.getMonitorPowerState({ edid });
      return res.responding ?? false;
    } catch {
      return false;
    }
  },

  saveMonitorSettings: async (settings: MonitorSettingsRequest) => {
    try {
      await client.default.setMonitorSettings(settings);
      // Re-fetch so capabilities/backend/tv reflect the saved change.
      await get().fetchMonitors(true);
      return true;
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed to save monitor settings");
      return false;
    }
  },

  setMonitorVolume: async (edid: string, volume: number) => {
    patchTVState(set, edid, { volume });
    try {
      await client.default.setMonitorVolume({ edid, volume });
    } catch {
      void get().fetchMonitors(true);
    }
  },

  setMonitorMute: async (edid: string, muted: boolean) => {
    patchTVState(set, edid, { muted });
    try {
      await client.default.setMonitorVolume({ edid, muted });
    } catch {
      void get().fetchMonitors(true);
    }
  },

  pairMonitor: async (edid: string) => {
    try {
      const data = await client.default.pairMonitor({ edid });
      if (data.success) {
        alert(data.message || "Pairing started — accept the prompt on the TV.");
        void get().fetchMonitors(true);
      } else {
        alert(data.message || "Pairing failed");
      }
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed to start pairing");
    }
  },
}));

// patchTVState optimistically merges fields into one monitor's tv_state, so
// sliders track the pointer without waiting for the next poll.
function patchTVState(
  set: (fn: (s: { monitors: Monitor[] }) => { monitors: Monitor[] }) => void,
  edid: string,
  patch: Partial<NonNullable<Monitor["tv_state"]>>,
) {
  set((s) => ({
    monitors: s.monitors.map((m) =>
      m.edid === edid && m.tv_state ? { ...m, tv_state: { ...m.tv_state, ...patch } } : m,
    ),
  }));
}
