/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
/**
 * Revision-guarded replacement of an absolute code-point range. selectionOnly changes the selection without replacing text.
 */
export type TrackpadMessageTextEdit = {
    type: 'textedit';
    fieldId: string;
    baseRevision: number;
    start: number;
    end: number;
    text: string;
    requestId: number;
    selectionOnly?: boolean;
};

