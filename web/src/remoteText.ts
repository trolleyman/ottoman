import type { TrackpadMessage, TrackpadMessageTextContext, TrackpadMessageTextEdit } from "./api";

export type TextContext = TrackpadMessageTextContext;
interface Pending { message: TrackpadMessageTextEdit; value: string }

// Only one revision-dependent edit may be in flight. Subsequent phone edits
// remain ordered; an external change or rejection replaces all optimistic state.
export class RemoteText {
  context: TextContext | null = null;
  view: TextContext | null = null;
  private pending: Pending[] = [];
  private nextId = 0;
  constructor(private send: (message: TrackpadMessage) => void) {}
  clear() { this.context = null; this.view = null; this.pending = []; }
  receive(next: TextContext) {
    const first = this.pending[0];
    if (first && next.requestId === first.message.requestId) {
      this.pending.shift();
      if (next.rejected || next.fieldId !== this.context?.fieldId || next.value !== first.value || next.windowStart !== this.context.windowStart) {
        this.pending = [];
      }
      this.context = next;
      if (this.pending.length) {
        this.pending[0].message.baseRevision = next.revision;
        this.send(this.pending[0].message);
      } else this.view = next;
      return;
    }
    if (first && next.fieldId === this.context?.fieldId && next.revision === this.context.revision) return;
    this.pending = [];
    this.context = next;
    this.view = next;
  }
  replace(start: number, end: number, text: string, selectionOnly = false) {
    const view = this.view;
    if (!view?.available || !view.editable || view.protected || view.password) return false;
    const chars = Array.from(view.value);
    const low = start - view.windowStart, high = end - view.windowStart;
    if (low < 0 || high < low || high > chars.length) return false;
    const value = selectionOnly ? view.value : chars.slice(0, low).join("") + text + chars.slice(high).join("");
    const caret = start + Array.from(text).length;
    this.view = { ...view, value, selectionStart: selectionOnly ? start : caret, selectionEnd: selectionOnly ? end : caret };
    const message: TrackpadMessageTextEdit = {
      type: "textedit", fieldId: view.fieldId, baseRevision: this.context!.revision,
      start, end, text, requestId: ++this.nextId, selectionOnly,
    };
    this.pending.push({ message, value });
    if (this.pending.length === 1) this.send(message);
    return true;
  }
}

export function codePointOffset(text: string, utf16Offset: number) {
  return Array.from(text.slice(0, utf16Offset)).length;
}
export function utf16Offset(text: string, codePoints: number) {
  return Array.from(text).slice(0, Math.max(0, codePoints)).join("").length;
}

// Wire offsets remain code points; a Backspace should remove the complete
// user-perceived character, including combining marks and joined emoji.
export function previousCharacterOffset(value: string, caret: number): number {
  const prefix = Array.from(value).slice(0, caret).join("");
  let boundary = 0;
  for (const part of new Intl.Segmenter(undefined, { granularity: "grapheme" }).segment(prefix)) boundary = part.index;
  return codePointOffset(prefix, boundary);
}

// Never mask by reading the protected value. Only explicit count metadata may
// control the mask; a fixed mask denotes password fields with unknown length.
export function textPreview(context: TextContext | null, fallback: string): string {
  if (context?.password) {
    const count = context.maskedLength;
    return "•".repeat(typeof count === "number" && Number.isFinite(count) && count >= 0
      ? Math.min(4096, Math.floor(count)) : 8);
  }
  if (context?.protected) return "";
  return context?.available ? context.value : fallback;
}

// Explain connection state without exposing field contents, names or errors.
export function textMirroringStatus(connected: boolean, context: TextContext | null): string {
  if (!connected) return "Desktop text: disconnected";
  if (!context) return "Desktop text: waiting for the agent";
  if (context.password) return "Desktop text: password field (contents hidden)";
  if (context.protected) return "Desktop text: access unavailable or screen locked";
  if (context.available) return "Desktop text: connected";
  return "Desktop text: no accessible text field detected";
}
