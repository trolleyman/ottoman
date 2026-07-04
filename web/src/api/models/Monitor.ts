/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ActiveMonitor } from './ActiveMonitor';
import type { MonitorCapabilities } from './MonitorCapabilities';
export type Monitor = {
    edid: string;
    port: string;
    name: string;
    manufacturer: string;
    active?: ActiveMonitor;
    friendly_name?: string;
    /**
     * ddc | tv | none
     */
    control_backend?: string;
    /**
     * current 0-100, -1 if unknown
     */
    brightness?: number;
    capabilities?: MonitorCapabilities;
    visibility?: Record<string, boolean>;
};

