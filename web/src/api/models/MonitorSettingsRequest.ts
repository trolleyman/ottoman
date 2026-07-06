/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { TVConn } from './TVConn';
export type MonitorSettingsRequest = {
    edid: string;
    friendly_name?: string;
    /**
     * ddc | i2c | tv | none
     */
    backend?: string;
    visibility?: Record<string, boolean>;
    tv?: TVConn;
};

