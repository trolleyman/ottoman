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

export interface ClientStatus {
  ip_address: string;
  mac_address: string;
  status: "online" | "offline" | "waking" | "shutting_down";
}

export interface StatusResponse {
  status: string;
  version: string;
  uptime: string;
  local_ip?: string;
  port?: string;
  secret?: string;
  client?: ClientStatus;
}

export interface WakeTarget {
  name: string;
  mac_address: string;
  ip_address: string;
  status?: string;
}

export type TrackpadRecvArgs = { t: "p"; x?: number; y?: number };

export type MouseButton = "left" | "right" | "middle" | "back" | "forward";

export type Modifier = "alt" | "ctrl" | "meta" | "shift";

export type TrackpadSendArgs =
  | { t: "m"; dx: number; dy: number }
  | { t: "c"; btn?: MouseButton; mod?: Modifier[] }
  | { t: "d"; btn?: MouseButton; mod?: Modifier[] }
  | { t: "u"; btn?: MouseButton; mod?: Modifier[] }
  | { t: "k"; text: string }
  | { t: "sc"; dx: number; dy: number; precise?: boolean }
  | { t: "key"; key: string; mod?: Modifier[] }
  | { t: "a"; x: number; y: number };
