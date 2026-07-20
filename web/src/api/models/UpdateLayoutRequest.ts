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
    /**
     * Replace the layout's monitor configuration with the display setup that is active right now, keeping its id, name, emoji and aliases. Lets an existing layout be re-captured after adjusting the displays, instead of having to delete it and save a new one.
     */
    capture_monitors?: boolean;
};

