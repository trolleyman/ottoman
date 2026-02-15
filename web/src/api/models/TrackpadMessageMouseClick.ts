/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { Modifier } from './Modifier';
import type { MouseButton } from './MouseButton';
export type TrackpadMessageMouseClick = {
    type: 'mouseclick';
    btn: MouseButton;
    modifiers: Array<Modifier>;
};

