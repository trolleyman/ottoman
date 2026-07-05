/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type UpdateLayoutRequest = {
    /**
     * ID of the layout to update
     */
    id: string;
    /**
     * New display name (optional; unchanged if omitted)
     */
    name?: string;
    /**
     * New emoji (optional; empty string clears it, omitted leaves unchanged)
     */
    emoji?: string;
    /**
     * Replacement alias list (optional; unchanged if omitted)
     */
    aliases?: Array<string>;
};

