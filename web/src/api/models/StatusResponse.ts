/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type StatusResponse = {
    /**
     * Operating system of the responding computer (Go GOOS, e.g. windows, linux, darwin).
     */
    os: string;
    status: string;
    version: string;
    uptime: string;
    hostname: string;
    ip_address: string;
    port: string;
    greeter?: boolean;
};

