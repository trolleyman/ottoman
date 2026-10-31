import type { TrackpadMessage, TrackpadMessageTextContext, TrackpadMessageTextEdit } from "./api";

export type TextContext = TrackpadMessageTextContext;
interface Pending { message: TrackpadMessageTextEdit; before: TextContext; after: TextContext }

// A browser may append a selected inline suggestion to the typed prefix. Only
// that exact suffix is safe to replace/rebase; arbitrary desktop changes are not.
function inlineCompletion(expected: TextContext, next: TextContext): boolean {
  if (!next.available || !next.editable || next.protected || next.password ||
    next.fieldId !== expected.fieldId || next.windowStart !== expected.windowStart ||
    next.selectionStart < next.windowStart || next.selectionStart !== expected.selectionStart || next.selectionEnd <= next.selectionStart) return false;
  const prefix = (context: TextContext) => Array.from(context.value).slice(0, context.selectionStart - context.windowStart).join("");
  return next.selectionEnd === next.windowStart + Array.from(next.value).length &&
    expected.selectionEnd === expected.windowStart + Array.from(expected.value).length &&
    prefix(next) === prefix(expected);
}

function replaceCompletion(pending: Pending, next: TextContext): boolean {
  // The next edit must end at the typed prefix (insert, backspace, or prefix
  // replacement). It will also consume the selected generated suffix.
  if (pending.message.selectionOnly || pending.message.end !== pending.before.selectionEnd ||
    pending.before.selectionStart !== next.selectionStart || pending.message.start > next.selectionStart) return false;
  pending.message = { ...pending.message, end: next.selectionEnd };
  pending.before = next;
  return true;
}

// Only one revision-dependent edit may be in flight. Subsequent phone edits
// remain ordered. Same-field notifications wait for the matching acknowledgment;
// the agent rechecks revisions before writing, so external edits still reject safely.
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
      const completed = inlineCompletion(first.after, next);
      // A suggestion can arrive after the previous acknowledgment but before
      // this edit is applied. Retry only an untouched typed prefix at a newer
      // revision; a real external edit, focus change or privacy change stops it.
      if (next.rejected && !completed && next.revision !== first.message.baseRevision &&
        inlineCompletion(first.before, next) && replaceCompletion(first, next)) {
        this.context = next;
        first.message.baseRevision = next.revision;
        first.message.requestId = ++this.nextId;
        this.send(first.message);
        return;
      }
      this.pending.shift();
      if ((!completed && (next.rejected || next.value !== first.after.value)) ||
        next.fieldId !== this.context?.fieldId || next.windowStart !== this.context.windowStart ||
        !next.available || !next.editable || next.protected || next.password) {
        this.pending = [];
      } else if (completed && this.pending.length && !replaceCompletion(this.pending[0], next)) {
        this.pending = [];
      }
      this.context = next;
      if (this.pending.length) {
        this.pending[0].message.baseRevision = next.revision;
        this.send(this.pending[0].message);
      } else this.view = next;
      return;
    }
    // Polling can observe an in-flight edit before its acknowledgment. Do not
    // replace newer phone input with that intermediate desktop value. Focus and
    // privacy changes still take effect immediately.
    if (first && next.fieldId === this.context?.fieldId && next.available &&
      next.editable && !next.protected && !next.password && !next.requestId) return;
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
    this.pending.push({ message, before: view, after: this.view });
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
