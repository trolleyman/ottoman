export interface MonitorInfo {
  edid: string;
  port: string;
  name: string;
  manufacturer: string;
  active?: ActiveMonitorInfo;
}

export interface ActiveMonitorInfo {
  width: number;
  height: number;
  refresh_rate: number;
  position_x: number;
  position_y: number;
  primary: boolean;
  model: string;
}

export interface LayoutMonitor {
  edid: string;
  port: string;
  name?: string;
  width: number;
  height: number;
  refresh_rate: number;
  position_x: number;
  position_y: number;
  primary: boolean;
}

export interface Layout {
  id: string;
  name: string;
  emoji?: string;
  aliases?: string[];
  monitors: LayoutMonitor[];
}

export interface LayoutsResponse {
  layouts: Layout[];
  current_layout: string;
}

export interface SwitchResponse {
  success: boolean;
  current_layout: string;
  message: string;
}

export interface WakeTarget {
  name: string;
  mac_address: string;
  ip_address: string;
  status?: string;
}

export type TrackpadRecvArgs = { t: "p"; x?: number; y?: number };

export type TrackpadSendArgs =
  | { t: "s"; touch: boolean }
  | { t: "m"; dx: number; dy: number }
  | { t: "e" }
  | { t: "c" }
  | { t: "k"; text: string }
  | { t: "a"; x: number; y: number };
