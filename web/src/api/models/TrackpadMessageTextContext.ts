/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
/**
 * Focused field window. All offsets are absolute Unicode code-point offsets. Unavailable or protected fields carry no text.
 */
export type TrackpadMessageTextContext = {
    type: 'textcontext';
    fieldId: string;
    revision: number;
    value: string;
    selectionStart: number;
    selectionEnd: number;
    windowStart: number;
    editable: boolean;
    multiline: boolean;
    available: boolean;
    protected: boolean;
    /**
     * A focused password field, distinct from a locked or inaccessible session. Its value and selection are always omitted.
     */
    password: boolean;
    /**
     * Optional password character count obtained without reading text. Omitted when unavailable; clients show a fixed mask.
     */
    maskedLength?: number;
    requestId?: number;
    rejected?: boolean;
};

