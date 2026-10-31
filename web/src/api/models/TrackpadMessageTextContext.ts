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
    requestId?: number;
    rejected?: boolean;
};

