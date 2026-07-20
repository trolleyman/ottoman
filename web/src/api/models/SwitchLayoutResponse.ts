/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type SwitchLayoutResponse = {
    success: boolean;
    current_layout: string;
    message?: string;
    /**
     * verified result of the switch
     */
    outcome?: SwitchLayoutResponse.outcome;
};
export namespace SwitchLayoutResponse {
    /**
     * verified result of the switch
     */
    export enum outcome {
        APPLIED = 'applied',
        ALREADY_ACTIVE = 'already-active',
        ROLLED_BACK = 'rolled-back',
        MISMATCH = 'mismatch',
        UNVERIFIED = 'unverified',
    }
}

