/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { AudioResponse } from '../models/AudioResponse';
import type { AudioSinksResponse } from '../models/AudioSinksResponse';
import type { AuthRequest } from '../models/AuthRequest';
import type { AuthResponse } from '../models/AuthResponse';
import type { LayoutsResponse } from '../models/LayoutsResponse';
import type { MonitorControlResponse } from '../models/MonitorControlResponse';
import type { MonitorSettingsRequest } from '../models/MonitorSettingsRequest';
import type { MonitorsResponse } from '../models/MonitorsResponse';
import type { RemoveLayoutRequest } from '../models/RemoveLayoutRequest';
import type { RemoveLayoutResponse } from '../models/RemoveLayoutResponse';
import type { SaveLayoutRequest } from '../models/SaveLayoutRequest';
import type { SaveLayoutResponse } from '../models/SaveLayoutResponse';
import type { SetAudioRequest } from '../models/SetAudioRequest';
import type { SetBrightnessRequest } from '../models/SetBrightnessRequest';
import type { SetMonitorPowerRequest } from '../models/SetMonitorPowerRequest';
import type { ShutdownResponse } from '../models/ShutdownResponse';
import type { SimSetStateRequest } from '../models/SimSetStateRequest';
import type { SimStateResponse } from '../models/SimStateResponse';
import type { StatusResponse } from '../models/StatusResponse';
import type { SwitchLayoutRequest } from '../models/SwitchLayoutRequest';
import type { SwitchLayoutResponse } from '../models/SwitchLayoutResponse';
import type { TVInputRequest } from '../models/TVInputRequest';
import type { TVPowerRequest } from '../models/TVPowerRequest';
import type { TVResponse } from '../models/TVResponse';
import type { TVStateResponse } from '../models/TVStateResponse';
import type { TVVolumeRequest } from '../models/TVVolumeRequest';
import type { WakeResponse } from '../models/WakeResponse';
import type { CancelablePromise } from '../core/CancelablePromise';
import type { BaseHttpRequest } from '../core/BaseHttpRequest';
export class DefaultService {
    constructor(public readonly httpRequest: BaseHttpRequest) {}
    /**
     * Health check
     * @returns string OK
     * @throws ApiError
     */
    public checkHealth(): CancelablePromise<string> {
        return this.httpRequest.request({
            method: 'GET',
            url: '/health',
        });
    }
    /**
     * Get system status
     * @returns StatusResponse OK
     * @throws ApiError
     */
    public getStatus(): CancelablePromise<StatusResponse> {
        return this.httpRequest.request({
            method: 'GET',
            url: '/api/status',
        });
    }
    /**
     * Get agent status
     * @returns StatusResponse OK
     * @throws ApiError
     */
    public getAgentStatus(): CancelablePromise<StatusResponse> {
        return this.httpRequest.request({
            method: 'GET',
            url: '/api/status/agent',
            errors: {
                401: `Unauthorized`,
                502: `Bad Gateway (Agent unreachable)`,
            },
        });
    }
    /**
     * Authenticate
     * @param requestBody
     * @returns AuthResponse Success
     * @throws ApiError
     */
    public auth(
        requestBody?: AuthRequest,
    ): CancelablePromise<AuthResponse> {
        return this.httpRequest.request({
            method: 'POST',
            url: '/api/auth',
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                401: `Unauthorized`,
            },
        });
    }
    /**
     * Logout
     * @returns AuthResponse Success
     * @throws ApiError
     */
    public logout(): CancelablePromise<AuthResponse> {
        return this.httpRequest.request({
            method: 'POST',
            url: '/api/auth/logout',
        });
    }
    /**
     * Check authentication status
     * @returns any Success
     * @throws ApiError
     */
    public checkAuth(): CancelablePromise<{
        /**
         * Whether the user is authenticated.
         */
        authenticated?: boolean;
    }> {
        return this.httpRequest.request({
            method: 'GET',
            url: '/api/auth/check',
            errors: {
                401: `Unauthorized`,
            },
        });
    }
    /**
     * Wake agent on LAN
     * @returns WakeResponse Success
     * @throws ApiError
     */
    public wake(): CancelablePromise<WakeResponse> {
        return this.httpRequest.request({
            method: 'POST',
            url: '/api/wake',
            errors: {
                401: `Unauthorized`,
                404: `No wake target configured`,
                500: `Internal Server Error`,
            },
        });
    }
    /**
     * Get all monitors
     * @returns MonitorsResponse Success
     * @throws ApiError
     */
    public getMonitors(): CancelablePromise<MonitorsResponse> {
        return this.httpRequest.request({
            method: 'GET',
            url: '/api/monitors',
            errors: {
                401: `Unauthorized`,
                502: `Bad Gateway (Agent unreachable)`,
            },
        });
    }
    /**
     * Get all layouts
     * @returns LayoutsResponse Success
     * @throws ApiError
     */
    public getLayouts(): CancelablePromise<LayoutsResponse> {
        return this.httpRequest.request({
            method: 'GET',
            url: '/api/layouts',
            errors: {
                401: `Unauthorized`,
                502: `Bad Gateway (Agent unreachable)`,
            },
        });
    }
    /**
     * Switch layout
     * @param requestBody
     * @returns SwitchLayoutResponse Success
     * @throws ApiError
     */
    public switchLayout(
        requestBody?: SwitchLayoutRequest,
    ): CancelablePromise<SwitchLayoutResponse> {
        return this.httpRequest.request({
            method: 'POST',
            url: '/api/layouts/switch',
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `Bad Request (Invalid body or ambiguous layout)`,
                401: `Unauthorized`,
                404: `Layout not found`,
                500: `Internal Server Error`,
                502: `Bad Gateway (Agent unreachable)`,
            },
        });
    }
    /**
     * Get current layout
     * @returns SwitchLayoutResponse Success
     * @throws ApiError
     */
    public getCurrentLayout(): CancelablePromise<SwitchLayoutResponse> {
        return this.httpRequest.request({
            method: 'GET',
            url: '/api/layouts/current',
            errors: {
                401: `Unauthorized`,
                502: `Bad Gateway (Agent unreachable)`,
            },
        });
    }
    /**
     * Save current layout
     * @param requestBody
     * @returns SaveLayoutResponse Success
     * @throws ApiError
     */
    public saveCurrentLayout(
        requestBody?: SaveLayoutRequest,
    ): CancelablePromise<SaveLayoutResponse> {
        return this.httpRequest.request({
            method: 'POST',
            url: '/api/layouts/save-current',
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `Bad Request (Invalid body or missing name)`,
                401: `Unauthorized`,
                500: `Internal Server Error`,
                502: `Bad Gateway (Agent unreachable)`,
            },
        });
    }
    /**
     * Remove layout
     * @param requestBody
     * @returns RemoveLayoutResponse Success
     * @throws ApiError
     */
    public removeLayout(
        requestBody?: RemoveLayoutRequest,
    ): CancelablePromise<RemoveLayoutResponse> {
        return this.httpRequest.request({
            method: 'POST',
            url: '/api/layouts/remove',
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `Bad Request (Invalid body)`,
                401: `Unauthorized`,
                404: `Layout not found`,
                500: `Internal Server Error`,
                502: `Bad Gateway (Agent unreachable)`,
            },
        });
    }
    /**
     * Shutdown agent
     * @returns ShutdownResponse Success
     * @throws ApiError
     */
    public shutdown(): CancelablePromise<ShutdownResponse> {
        return this.httpRequest.request({
            method: 'POST',
            url: '/api/shutdown',
            errors: {
                401: `Unauthorized`,
                502: `Bad Gateway (Agent unreachable)`,
            },
        });
    }
    /**
     * List audio output sinks
     * @returns AudioSinksResponse Success
     * @throws ApiError
     */
    public getAudioSinks(): CancelablePromise<AudioSinksResponse> {
        return this.httpRequest.request({
            method: 'GET',
            url: '/api/audio/sinks',
            errors: {
                401: `Unauthorized`,
                500: `Internal Server Error (audio unavailable)`,
                502: `Bad Gateway (Agent unreachable)`,
            },
        });
    }
    /**
     * Set an audio sink's volume, mute, or default state
     * @param requestBody
     * @returns AudioResponse Success
     * @throws ApiError
     */
    public setAudioVolume(
        requestBody?: SetAudioRequest,
    ): CancelablePromise<AudioResponse> {
        return this.httpRequest.request({
            method: 'POST',
            url: '/api/audio/volume',
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `Bad Request`,
                401: `Unauthorized`,
                500: `Internal Server Error`,
                502: `Bad Gateway (Agent unreachable)`,
            },
        });
    }
    /**
     * Set a monitor's brightness
     * @param requestBody
     * @returns MonitorControlResponse Success
     * @throws ApiError
     */
    public setMonitorBrightness(
        requestBody?: SetBrightnessRequest,
    ): CancelablePromise<MonitorControlResponse> {
        return this.httpRequest.request({
            method: 'POST',
            url: '/api/monitors/brightness',
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `Bad Request`,
                401: `Unauthorized`,
                500: `Internal Server Error`,
                502: `Bad Gateway (Agent unreachable)`,
            },
        });
    }
    /**
     * Turn a monitor on or off (standby)
     * @param requestBody
     * @returns MonitorControlResponse Success
     * @throws ApiError
     */
    public setMonitorPower(
        requestBody?: SetMonitorPowerRequest,
    ): CancelablePromise<MonitorControlResponse> {
        return this.httpRequest.request({
            method: 'POST',
            url: '/api/monitors/power',
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `Bad Request`,
                401: `Unauthorized`,
                500: `Internal Server Error`,
                502: `Bad Gateway (Agent unreachable)`,
            },
        });
    }
    /**
     * Update a monitor's registry settings (name, backend, visibility)
     * @param requestBody
     * @returns MonitorControlResponse Success
     * @throws ApiError
     */
    public setMonitorSettings(
        requestBody?: MonitorSettingsRequest,
    ): CancelablePromise<MonitorControlResponse> {
        return this.httpRequest.request({
            method: 'POST',
            url: '/api/monitors/settings',
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `Bad Request`,
                401: `Unauthorized`,
                500: `Internal Server Error`,
                502: `Bad Gateway (Agent unreachable)`,
            },
        });
    }
    /**
     * Get TV integration state
     * @returns TVStateResponse Success
     * @throws ApiError
     */
    public getTvState(): CancelablePromise<TVStateResponse> {
        return this.httpRequest.request({
            method: 'GET',
            url: '/api/tv/state',
            errors: {
                401: `Unauthorized`,
                502: `Bad Gateway (Agent unreachable)`,
            },
        });
    }
    /**
     * Start TV on-screen pairing
     * @returns TVResponse Success
     * @throws ApiError
     */
    public pairTv(): CancelablePromise<TVResponse> {
        return this.httpRequest.request({
            method: 'POST',
            url: '/api/tv/pair',
            errors: {
                401: `Unauthorized`,
                500: `Internal Server Error`,
                502: `Bad Gateway (Agent unreachable)`,
            },
        });
    }
    /**
     * Turn the TV on (Wake-on-LAN) or off (SSAP)
     * @param requestBody
     * @returns TVResponse Success
     * @throws ApiError
     */
    public setTvPower(
        requestBody?: TVPowerRequest,
    ): CancelablePromise<TVResponse> {
        return this.httpRequest.request({
            method: 'POST',
            url: '/api/tv/power',
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `Bad Request`,
                401: `Unauthorized`,
                500: `Internal Server Error`,
                502: `Bad Gateway (Agent unreachable)`,
            },
        });
    }
    /**
     * Set the TV volume and/or mute
     * @param requestBody
     * @returns TVResponse Success
     * @throws ApiError
     */
    public setTvVolume(
        requestBody?: TVVolumeRequest,
    ): CancelablePromise<TVResponse> {
        return this.httpRequest.request({
            method: 'POST',
            url: '/api/tv/volume',
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `Bad Request`,
                401: `Unauthorized`,
                500: `Internal Server Error`,
                502: `Bad Gateway (Agent unreachable)`,
            },
        });
    }
    /**
     * Switch the TV's external input
     * @param requestBody
     * @returns TVResponse Success
     * @throws ApiError
     */
    public setTvInput(
        requestBody?: TVInputRequest,
    ): CancelablePromise<TVResponse> {
        return this.httpRequest.request({
            method: 'POST',
            url: '/api/tv/input',
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `Bad Request`,
                401: `Unauthorized`,
                500: `Internal Server Error`,
                502: `Bad Gateway (Agent unreachable)`,
            },
        });
    }
    /**
     * Reset simulation
     * @returns SimStateResponse Success
     * @throws ApiError
     */
    public simReset(): CancelablePromise<SimStateResponse> {
        return this.httpRequest.request({
            method: 'POST',
            url: '/api/sim/reset',
            errors: {
                404: `Not Found (Controller not in simulation mode)`,
            },
        });
    }
    /**
     * Get simulation state
     * @returns SimStateResponse Success
     * @throws ApiError
     */
    public simState(): CancelablePromise<SimStateResponse> {
        return this.httpRequest.request({
            method: 'GET',
            url: '/api/sim/state',
            errors: {
                404: `Not Found (Controller not in simulation mode)`,
            },
        });
    }
    /**
     * Set simulation state
     * @param requestBody
     * @returns SimStateResponse Success
     * @throws ApiError
     */
    public simSetState(
        requestBody?: SimSetStateRequest,
    ): CancelablePromise<SimStateResponse> {
        return this.httpRequest.request({
            method: 'POST',
            url: '/api/sim/set-state',
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                404: `Not Found (Controller not in simulation mode)`,
            },
        });
    }
    /**
     * WebSocket Trackpad Connection
     * Upgrades to WebSocket. Messages follow TrackpadSendArgs/TrackpadRecvArgs schemas.
     * @returns void
     * @throws ApiError
     */
    public connectTrackpad(): CancelablePromise<void> {
        return this.httpRequest.request({
            method: 'GET',
            url: '/api/trackpad',
        });
    }
}
