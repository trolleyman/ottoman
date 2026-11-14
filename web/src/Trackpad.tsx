import { createPortal } from "react-dom";
import { useState, useEffect, useCallback, useRef } from "react";
import {
  Eraser,
  Settings,
  X,
} from "lucide-react";
import { ScrollRail } from "./ScrollRail";
import { MonitorDisplay } from "./MonitorDisplay";
import { useStore } from "./store";
import { Modifier, MouseButton, type TrackpadMessage, type TrackpadMessageTextContext } from "./api";
import { RemoteText, codePointOffset, utf16Offset, previousCharacterOffset, textPreview, textMirroringStatus } from "./remoteText";
import { characterCount, diffText } from "./trackpadText";
import { KeyboardModifiers, spaceEdit } from "./keyboardEditing";
import { previewHit, previewMove, previewPositions } from "./previewNavigation";
import { CaretText } from "./CaretText";
import { CustomKeyboard } from "./CustomKeyboard";
import { TRACKPAD_RANGES, type TrackpadSettings } from "./trackpadSettings";
import { loadSettings, savePreference } from "./preferences";

function getModifiers(e: {
  shiftKey: boolean;
  ctrlKey: boolean;
  altKey: boolean;
  metaKey: boolean;
}) {
  const mod: Modifier[] = [];
  if (e.shiftKey) mod.push(Modifier.SHIFT);
  if (e.ctrlKey) mod.push(Modifier.CTRL);
  if (e.altKey) mod.push(Modifier.ALT);
  if (e.metaKey) mod.push(Modifier.META);
  return mod.length > 0 ? mod : [];
}

function combineModifiers(...groups: Modifier[][]): Modifier[] {
  return [...new Set(groups.flat())];
}

function TouchArea({
  sideControls,
  docked = false,
  textContext,
  connected,
  connecting,
  send,
  settings,
  onSettingsChange,
}: {
  textContext: TrackpadMessageTextContext | null;
  connected: boolean;
  connecting: boolean;
  send: (msg: TrackpadMessage) => void;
  sideControls?: React.ReactNode;
  docked?: boolean;
  settings: TrackpadSettings;
  onSettingsChange: (patch: Partial<TrackpadSettings>) => void;
}) {
  const [typingContext, setTypingContext] = useState(0);
  const [remote, setRemote] = useState<TrackpadMessageTextContext | null>(null);
  const remoteModel = useRef<RemoteText | null>(null);
  if (remoteModel.current == null) { remoteModel.current = new RemoteText(send); }
  const navigationAnchor = useRef<number | null>(null);
  const navigationX = useRef<number | undefined>(undefined);
  const typingMemory = useRef<{ field?: string; period: number | null; suppress: boolean; space: number | null }>({ period: null, suppress: false, space: null });
  const dragOrigin = useRef<{ x: number; y: number; left: number; top: number; height: number; scaleX: number; scaleY: number } | null>(null);
  const [cursorDrag, setCursorDrag] = useState<{ left: number; top: number; height: number } | null>(null);
  const composing = useRef(false);
  const compositionContext = useRef<{fieldId: string; revision: number} | null>(null);
  const [compositionValue, setCompositionValue] = useState<string | null>(null);
  useEffect(() => {
    remoteModel.current!.clear();
    // Reset the preview when the external WebSocket subscription changes.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setRemote(null);
    if (connected) send({ type: "textwatch", enabled: settings.mirrorFocusedText });
    return () => { remoteModel.current!.clear(); };
  }, [connected, settings.mirrorFocusedText, send]);
  useEffect(() => {
    if (!settings.mirrorFocusedText || !connected || !textContext) return;
    const previous = remoteModel.current!.view;
    remoteModel.current!.receive(textContext);
    const next = remoteModel.current!.view;
    // Own acknowledged edits preserve the optimistic value/caret. External
    // changes invalidate swipe alternatives and deferred word separators.
    if (previous && next && (previous.fieldId !== next.fieldId || previous.value !== next.value ||
      previous.selectionStart !== next.selectionStart || previous.selectionEnd !== next.selectionEnd ||
      previous.windowStart !== next.windowStart || previous.password !== next.password || previous.protected !== next.protected)) {
      setTypingContext((value) => value + 1);
      navigationAnchor.current = null;
      navigationX.current = undefined;
      if (previous.value !== next.value) typingMemory.current.space = null;
    }
    setRemote(next);
  }, [textContext, settings.mirrorFocusedText, connected]);
  const replaceRemote = useCallback((start: number, end: number, text: string, selectionOnly = false) => {
    const applied = remoteModel.current!.replace(start, end, text, selectionOnly);
    if (applied) setRemote(remoteModel.current!.view);
    return applied;
  }, []);
  const trackpadRef = useRef<HTMLDivElement>(null);
  const lastTouchRef = useRef<{ x: number; y: number } | null>(null);
  const lastMoveTime = useRef(0);
  const touchStartTime = useRef(0);
  const touchStartPos = useRef<{ x: number; y: number } | null>(null);
  const lastTouchEndTime = useRef(0);
  const isDragging = useRef(false);
  const mouseHeld = useRef(false);
  const dragLocked = useRef(false);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const [focused, setFocused] = useState(false);
  const [keyboardOpen, setKeyboardOpen] = useState(false);
  const [keyboardSettingsOpen, setKeyboardSettingsOpen] = useState(false);
  const [activeModifiers, setActiveModifiers] = useState<Modifier[]>([]);
  const modifierState = useRef(new KeyboardModifiers());
  const [lockedModifiers, setLockedModifiers] = useState<Modifier[]>([]);
  const syncModifiers = useCallback(() => {
    setActiveModifiers(modifierState.current.active);
    setLockedModifiers([...modifierState.current.states].filter(([, state]) => state === "locked").map(([key]) => key));
  }, []);
  const consumeModifiers = useCallback(() => { modifierState.current.consume(); syncModifiers(); }, [syncModifiers]);
  const previewRef = useRef<HTMLDivElement>(null);
  const tapReleaseTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const doubleClickTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const tapMouseDown = useRef(false);
  const secondTap = useRef(false);
  const [draft, setDraft] = useState("");
  const [draftCursor, setDraftCursor] = useState(0);
  const [draftSelection, setDraftSelection] = useState<[number, number] | null>(null);
  const [draftVisible, setDraftVisible] = useState(false);
  const draftRef = useRef("");
  const remoteCursor = useRef(0);
  // Fallback mouse position tracking when pointer lock is unavailable (e.g. after Escape cooldown)
  const lastMousePos = useRef<{ x: number; y: number } | null>(null);

  // Two-finger scroll refs
  const twoFingerRef = useRef(false);
  const twoFingerLastX = useRef(0);
  const twoFingerLastY = useRef(0);
  const twoFingerStartTime = useRef(0);
  const twoFingerMoved = useRef(false);
  const scrollPrimed = useRef(false);

  // Where the previous single-finger gesture ended, and whether that gesture was
  // one that a following quick touch should turn into a click-and-drag (a genuine
  // tap, or an active drag being continued). A plain cursor move is neither, so
  // moving then quickly moving again keeps moving instead of starting a drag.
  const lastTapEndPos = useRef<{ x: number; y: number } | null>(null);
  const lastGestureAllowsDrag = useRef(false);
  const gestureMoved = useRef(false);
  const scrollVelocity = useRef<{ x: number; y: number }>({ x: 0, y: 0 });
  const inertiaFrame = useRef(0);
  const lastScrollTime = useRef(0);

  // Cursor inertia refs
  const cursorVelocity = useRef<{ x: number; y: number }>({ x: 0, y: 0 });
  const cursorInertiaFrame = useRef(0);
  const lastCursorTime = useRef(0);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent | KeyboardEvent) => {
      // Skip unidentified keys - handled by input event on mobile
      if (e.key === "Unidentified" || e.key === "Process") return;

      e.preventDefault();
      e.stopPropagation();

      send({
        type: "keydown",
        key: e.key,
        modifiers: combineModifiers(getModifiers(e), activeModifiers),
      });
    },
    [activeModifiers, send],
  );

  // Release keys pressed via handleKeyDown (physical keyboard). Without a keyup
  // the key stays held on the host's virtual device and wedges input.
  const handleKeyUp = useCallback(
    (e: React.KeyboardEvent | KeyboardEvent) => {
      if (e.key === "Unidentified" || e.key === "Process") return;

      e.preventDefault();
      e.stopPropagation();

      send({
        type: "keyup",
        key: e.key,
        modifiers: combineModifiers(getModifiers(e), activeModifiers),
      });
    },
    [activeModifiers, send],
  );

  const sendKeyPress = useCallback(
    (key: string, modifiers: Modifier[] = []) => {
      send({ type: "keydown", key, modifiers });
      send({ type: "keyup", key, modifiers });
      consumeModifiers();
    },
    [send, consumeModifiers],
  );

  const showDraftIndicator = useCallback(() => setDraftVisible(true), []);
  const dismissDraft = useCallback(() => {
    dragOrigin.current = null;
    setCursorDrag(null);
    setTypingContext((value) => value + 1);
    setDraftVisible(false);
    draftRef.current = "";
    remoteCursor.current = 0;
    setDraft("");
    setDraftCursor(0);
    setDraftSelection(null);
  }, []);
  const remoteOS = useStore((state) => state.agentInfo?.os ?? "");

  useEffect(() => {
    if (!keyboardOpen || docked) return;
    const frame = requestAnimationFrame(() => {
      if (matchMedia("(max-width: 767px)").matches)
        trackpadRef.current?.scrollIntoView({ block: "start", behavior: "smooth" });
    });
    return () => cancelAnimationFrame(frame);
  }, [keyboardOpen, docked]);

  useEffect(() => {
    const releaseHeld = () => {
      for (const key of modifierState.current.held.keys()) modifierState.current.release(key, 0, 0, true);
      syncModifiers();
    };
    window.addEventListener("blur", releaseHeld);
    return () => window.removeEventListener("blur", releaseHeld);
  }, [syncModifiers]);

  const beginModifier = (modifier: Modifier) => { modifierState.current.press(modifier); syncModifiers(); };
  const endModifier = (modifier: Modifier, cancelled = false) => {
    modifierState.current.release(modifier, performance.now(), settings.shiftDoubleTapMs, cancelled);
    syncModifiers();
  };

  const moveRemoteCursor = useCallback(
    (target: number) => {
      const delta = target - remoteCursor.current;
      const key = delta < 0 ? "ArrowLeft" : "ArrowRight";
      for (let i = 0; i < Math.abs(delta); i++) sendKeyPress(key);
      remoteCursor.current = target;
    },
    [sendKeyPress],
  );

  useEffect(() => {
    draftRef.current = "";
    remoteCursor.current = 0;
    // Native focus changes invalidate the fallback input history.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setDraft("");
    setDraftCursor(0);
    setDraftSelection(null);
    setCompositionValue(null);
    setTypingContext((value) => value + 1);
  }, [connected, settings.mirrorFocusedText, textContext?.fieldId, textContext?.protected, textContext?.password]);
  useEffect(() => {
    // A native text/caret notification is activity even without phone input.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (remote?.available || remote?.password) showDraftIndicator();
  }, [remote, showDraftIndicator]);
  useEffect(() => {
    if (!remote?.available || composing.current || !inputRef.current) return;
    const input = inputRef.current;
    const start = utf16Offset(remote.value, remote.selectionStart - remote.windowStart);
    const end = utf16Offset(remote.value, remote.selectionEnd - remote.windowStart);
    // Reapplying an unchanged range resets the browser's selection anchor;
    // repeated Shift+Left would alternate between extending and collapsing it.
    if (input.selectionStart !== start || input.selectionEnd !== end) {
      input.setSelectionRange(start, end, input.selectionDirection);
    }
  }, [remote]);

  const handleInputKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.nativeEvent.isComposing || composing.current) return;
      if (remoteModel.current!.view?.editable && e.key.startsWith("Arrow")) return;
      if (
        e.key === "Unidentified" ||
        e.key === "Process" ||
        e.key.length === 1 ||
        e.key === "Backspace"
      ) return;

      e.preventDefault();
      e.stopPropagation();
      sendKeyPress(e.key, combineModifiers(getModifiers(e), activeModifiers));
      if (e.key === "Enter") dismissDraft();
    },
    [sendKeyPress, activeModifiers, dismissDraft],
  );

  const handleTextChange = useCallback(
    (e: React.ChangeEvent<HTMLTextAreaElement>) => {
      const next = e.currentTarget.value;
      if (composing.current) { setCompositionValue(next); return; }
      // Some phone keyboards report Return as an input event with no keydown.
      const inputType = (e.nativeEvent as InputEvent).inputType;
      if (inputType === "insertLineBreak" || inputType === "insertParagraph") {
        sendKeyPress("Enter", activeModifiers);
        dismissDraft();
        return;
      }
      const view = remoteModel.current!.view;
      if (view?.available && view.editable) {
        const edit = diffText(view.value, next);
        if (edit) replaceRemote(
          view.windowStart + codePointOffset(view.value, edit.start),
          view.windowStart + codePointOffset(view.value, edit.start + edit.removed.length), edit.inserted,
        );
        return;
      }
      const edit = diffText(draftRef.current, next);
      if (edit) {
        const editStart = characterCount(draftRef.current.slice(0, edit.start));
        const removedCount = characterCount(edit.removed);
        moveRemoteCursor(editStart + removedCount);
        for (let i = 0; i < removedCount; i++) sendKeyPress("Backspace");
        remoteCursor.current = editStart;
        if (edit.inserted) {
          send({ type: "text", text: edit.inserted });
          remoteCursor.current += characterCount(edit.inserted);
        }
      }

      draftRef.current = next;
      setDraft(next);
      setDraftCursor(e.currentTarget.selectionEnd);
      showDraftIndicator();
      moveRemoteCursor(characterCount(next.slice(0, e.currentTarget.selectionEnd)));
    },
    [moveRemoteCursor, replaceRemote, send, sendKeyPress, showDraftIndicator, activeModifiers, dismissDraft],
  );

  const handleSelection = useCallback(
    (e: React.SyntheticEvent<HTMLTextAreaElement>) => {
      const input = e.currentTarget;
      if (composing.current) return;
      const view = remoteModel.current!.view;
      if (view?.available && view.editable) {
        // A delayed selection event can describe an older controlled value.
        if (input.value !== view.value) return;
        const start = view.windowStart + codePointOffset(input.value, input.selectionStart);
        const end = view.windowStart + codePointOffset(input.value, input.selectionEnd);
        if (start !== view.selectionStart || end !== view.selectionEnd) replaceRemote(start, end, "", true);
        return;
      }
      if (input.selectionStart === input.selectionEnd) {
        setDraftCursor(input.selectionEnd);
        showDraftIndicator();
        moveRemoteCursor(characterCount(input.value.slice(0, input.selectionEnd)));
      }
    },
    [moveRemoteCursor, replaceRemote, showDraftIndicator],
  );

  const editFallback = useCallback((start: number, end: number, text: string) => {
    const chars = Array.from(draftRef.current);
    if (draftSelection) { sendKeyPress("ArrowRight"); remoteCursor.current = Math.max(...draftSelection); }
    moveRemoteCursor(end);
    for (let i = start; i < end; i++) sendKeyPress("Backspace");
    if (text) send({ type: "text", text });
    const next = chars.slice(0, start).join("") + text + chars.slice(end).join("");
    draftRef.current = next;
    remoteCursor.current = start + characterCount(text);
    setDraft(next);
    setDraftCursor(utf16Offset(next, remoteCursor.current));
    setDraftSelection(null);
    showDraftIndicator();
  }, [moveRemoteCursor, sendKeyPress, send, showDraftIndicator, draftSelection]);

  const updateNativePreview = useCallback((start: number, end: number, text: string) => {
    const view = remoteModel.current!.view;
    if (!view?.available || view.editable || view.password || view.protected) return;
    const chars = Array.from(view.value);
    const value = chars.slice(0, start - view.windowStart).join("") + text + chars.slice(end - view.windowStart).join("");
    const caret = start + characterCount(text);
    remoteModel.current!.view = { ...view, value, selectionStart: caret, selectionEnd: caret };
    setRemote(remoteModel.current!.view);
  }, []);

  const appendText = useCallback((value: string, shifted = false, suggested = false) => {
    const modifiers = modifierState.current.active;
    if (modifiers.some(modifier => modifier !== Modifier.SHIFT) && characterCount(value) === 1) {
      sendKeyPress(value.toLocaleLowerCase(), combineModifiers(modifiers, shifted ? [Modifier.SHIFT] : []));
      return;
    }
    const view = remoteModel.current!.view;
    const memory = typingMemory.current;
    if (memory.field !== view?.fieldId) Object.assign(memory, { field: view?.fieldId, period: null, suppress: false, space: null });
    const position = view?.available ? view.selectionStart : draftSelection ? Math.min(...draftSelection) : remoteCursor.current;
    const end = view?.available ? view.selectionEnd : draftSelection ? Math.max(...draftSelection) : position;
    const before = Array.from(view?.available ? view.value : draftRef.current).slice(0, position - (view?.available ? view.windowStart : 0)).join("");
    const edit = spaceEdit(before, value, settings.doubleSpacePeriod && position === end, memory.suppress, memory.space === position);
    if (edit.text === ". " && value === " " && edit.remove) memory.period = position - 1;
    if (/\S/.test(value)) memory.suppress = false;
    memory.space = suggested ? position - edit.remove + characterCount(edit.text) : null;
    if (!replaceRemote(position - edit.remove, end, edit.text)) {
      if (view?.available) {
        // Rich editors expose a preview but require native keystrokes.
        for (let i = 0; i < edit.remove; i++) sendKeyPress("Backspace");
        send({ type: "text", text: edit.text });
        updateNativePreview(position - edit.remove, end, edit.text);
      } else editFallback(position - edit.remove, end, edit.text);
    }
    navigationAnchor.current = null;
    navigationX.current = undefined;
    consumeModifiers();
    showDraftIndicator();
  }, [sendKeyPress, settings.doubleSpacePeriod, replaceRemote, send, editFallback, consumeModifiers, showDraftIndicator, draftSelection, updateNativePreview]);

  const backspace = useCallback(() => {
    const view = remoteModel.current!.view;
    const modifiers = modifierState.current.active;
    const position = view?.available ? view.selectionStart : remoteCursor.current;
    const memory = typingMemory.current;
    if (memory.period === position - 1 && Array.from(view?.available ? view.value : draftRef.current)[position - 1 - (view?.available ? view.windowStart : 0)] === ".") { memory.suppress = true; memory.period = null; }
    memory.space = null;
    if (view?.available && view.editable && modifiers.length === 0) {
      replaceRemote(view.selectionStart === view.selectionEnd ? view.windowStart + previousCharacterOffset(view.value, position - view.windowStart) : position, view.selectionEnd, "");
    } else if (!view?.available && modifiers.length === 0) {
      if (position === 0 && !draftSelection) sendKeyPress("Backspace");
      editFallback(draftSelection ? Math.min(...draftSelection) : previousCharacterOffset(draftRef.current, position), draftSelection ? Math.max(...draftSelection) : position, "");
    } else {
      sendKeyPress("Backspace", modifiers);
      if (view?.available && !modifiers.some(modifier => modifier !== Modifier.SHIFT)) updateNativePreview(view.selectionStart === view.selectionEnd ? view.windowStart + previousCharacterOffset(view.value, position - view.windowStart) : position, view.selectionEnd, "");
    }
    navigationAnchor.current = null;
  }, [replaceRemote, editFallback, sendKeyPress, draftSelection, updateNativePreview]);

  const selectPreview = (offset: number, extend = false) => {
    const view = remoteModel.current!.view;
    if (view?.password || view?.protected) return false;
    if (view?.available) {
      const target = view.windowStart + codePointOffset(view.value, offset);
      if (view.editable) {
        const anchor = extend ? navigationAnchor.current ?? view.selectionStart : target;
        navigationAnchor.current = extend ? anchor : null;
        return replaceRemote(Math.min(anchor, target), Math.max(anchor, target), "", true);
      }
      // Expanded rich text cannot be edited with accessibility offsets; native
      // horizontal steps still correspond to characters in the preview.
      const anchor = extend ? navigationAnchor.current ?? view.selectionStart : target;
      const current = extend && navigationAnchor.current === view.selectionEnd ? view.selectionStart : view.selectionEnd;
      if (!extend && view.selectionStart !== view.selectionEnd) sendKeyPress("ArrowRight");
      const delta = target - current;
      for (let i = 0; i < Math.abs(delta); i++) sendKeyPress(delta < 0 ? "ArrowLeft" : "ArrowRight", extend ? [Modifier.SHIFT] : []);
      navigationAnchor.current = extend ? anchor : null;
      remoteModel.current!.view = { ...view, selectionStart: Math.min(anchor, target), selectionEnd: Math.max(anchor, target) };
      setRemote(remoteModel.current!.view);
      return true;
    }
    const target = codePointOffset(draftRef.current, offset);
    if (extend) {
      const anchor = navigationAnchor.current ?? remoteCursor.current;
      navigationAnchor.current = anchor;
      const delta = target - remoteCursor.current;
      for (let i = 0; i < Math.abs(delta); i++) sendKeyPress(delta < 0 ? "ArrowLeft" : "ArrowRight", [Modifier.SHIFT]);
      remoteCursor.current = target;
      setDraftSelection([anchor, target]);
    } else {
      if (draftSelection) { sendKeyPress("ArrowRight"); remoteCursor.current = Math.max(...draftSelection); }
      moveRemoteCursor(target);
      navigationAnchor.current = null;
      setDraftSelection(null);
    }
    setDraftCursor(offset);
    return true;
  };

  const navigate = (key: string) => {
    const view = remoteModel.current!.view;
    const modifiers = modifierState.current.active;
    if (settings.previewNavigation && previewRef.current && !view?.password && !view?.protected &&
      !modifiers.some(modifier => modifier !== Modifier.SHIFT) && /^(Arrow(Up|Down|Left|Right)|Home|End|PageUp|PageDown)$/.test(key)) {
      const value = view?.available ? view.value : draftRef.current;
      const extending = modifiers.includes(Modifier.SHIFT);
      const anchor = navigationAnchor.current;
      if (!extending && (key === "ArrowLeft" || key === "ArrowRight")) {
        const selected = view?.available ? [view.selectionStart, view.selectionEnd] : draftSelection;
        if (selected && selected[0] !== selected[1]) {
          selectPreview(utf16Offset(value, (key === "ArrowLeft" ? Math.min(...selected) : Math.max(...selected)) - (view?.available ? view.windowStart : 0)));
          consumeModifiers(); return;
        }
      }
      const position = view?.available ? (extending && anchor === view.selectionEnd ? view.selectionStart : view.selectionEnd) - view.windowStart : remoteCursor.current;
      const result = previewMove(previewRef.current, utf16Offset(value, position), key, navigationX.current);
      if (result) {
        navigationX.current = result.x;
        if (selectPreview(result.offset, extending)) { consumeModifiers(); showDraftIndicator(); return; }
      }
    }
    navigationAnchor.current = null;
    navigationX.current = undefined;
    sendKeyPress(key, modifiers);
    showDraftIndicator();
  };

  const handleCursorDrag = (phase: "start" | "move" | "end", x: number, y: number) => {
    const root = previewRef.current;
    const view = remoteModel.current!.view;
    if (phase === "end") { dragOrigin.current = null; setCursorDrag(null); return false; }
    if (!root || view?.password || view?.protected) return false;
    const bounds = root.getBoundingClientRect();
    if (phase === "start") {
      const value = view?.available ? view.value : draftRef.current;
      const offset = view?.available ? utf16Offset(value, view.selectionEnd - view.windowStart) : utf16Offset(value, remoteCursor.current);
      const positions = previewPositions(root);
      const point = positions.find(point => point.offset === offset);
      if (!point) return false;
      const caret = root.querySelector<HTMLElement>("[data-preview-caret]")?.getBoundingClientRect();
      const left = (caret?.left ?? point.x) - bounds.left;
      const top = (caret?.top ?? point.y) - bounds.top;
      const height = caret?.height || point.height || 20;
      const widths = positions.slice(1).flatMap((point, index) => {
        const previous = positions[index];
        const width = point.x - previous.x;
        return Math.abs(point.y - previous.y) < 2 && width > 0 ? [width] : [];
      });
      const characterWidth = widths.length ? widths.reduce((a, b) => a + b, 0) / widths.length : 8;
      dragOrigin.current = { x, y, left, top, height, scaleX: characterWidth * settings.spaceCursorSensitivity / 12, scaleY: 20 * settings.spaceCursorSensitivity / 30 };
      setCursorDrag({ left, top, height });
      showDraftIndicator();
      return settings.previewNavigation;
    }
    const origin = dragOrigin.current;
    if (!origin) return false;
    const viewport = root.closest<HTMLElement>("[data-preview-scroll]")!.getBoundingClientRect();
    const left = Math.max(4, Math.min(bounds.width - 4, origin.left + (x - origin.x) * origin.scaleX));
    const top = Math.max(viewport.top - bounds.top, Math.min(viewport.bottom - bounds.top - origin.height, origin.top + (y - origin.y) * origin.scaleY));
    setCursorDrag({ left, top, height: origin.height });
    const modifiers = modifierState.current.active;
    if (!settings.previewNavigation || modifiers.some(modifier => modifier !== Modifier.SHIFT)) return false;
    const offset = previewHit(root, bounds.left + left, bounds.top + top + origin.height / 2);
    if (offset !== undefined) {
      const value = view?.available ? view.value : draftRef.current;
      const current = view?.available ? utf16Offset(value, (navigationAnchor.current === view.selectionEnd ? view.selectionStart : view.selectionEnd) - view.windowStart) : utf16Offset(value, remoteCursor.current);
      if (offset !== current) {
        selectPreview(offset, modifiers.includes(Modifier.SHIFT));
        consumeModifiers();
      }
    }
    return true;
  };

  useEffect(() => {
    const release = () => {
      if (tapReleaseTimer.current) clearTimeout(tapReleaseTimer.current);
      if (doubleClickTimer.current) clearTimeout(doubleClickTimer.current);
      tapReleaseTimer.current = null; doubleClickTimer.current = null;
      secondTap.current = false;
      if (tapMouseDown.current) send({ type: "mouseup", btn: MouseButton.LEFT, modifiers: [] });
      tapMouseDown.current = false; isDragging.current = false;
    };
    window.addEventListener("blur", release);
    return () => { window.removeEventListener("blur", release); release(); };
  }, [send, connected]);

  const releaseTap = () => {
    if (tapReleaseTimer.current) clearTimeout(tapReleaseTimer.current);
    tapReleaseTimer.current = null;
    if (tapMouseDown.current) send({ type: "mouseup", btn: MouseButton.LEFT, modifiers: activeModifiers });
    tapMouseDown.current = false;
  };

  const activateKeyboard = () => {
    if (settings.customKeyboard) setKeyboardOpen(true);
    else inputRef.current?.focus();
  };

  const onTouchStart = (e: React.TouchEvent) => {
    if (doubleClickTimer.current) {
      clearTimeout(doubleClickTimer.current); doubleClickTimer.current = null;
      send({ type: "mouseclick", btn: MouseButton.LEFT, modifiers: activeModifiers });
    }
    e.preventDefault();
    dismissDraft();

    // Stop any active inertia scrolling when the user touches the screen again
    if (inertiaFrame.current) {
      cancelAnimationFrame(inertiaFrame.current);
      inertiaFrame.current = 0;
    }
    if (cursorInertiaFrame.current) {
      cancelAnimationFrame(cursorInertiaFrame.current);
      cursorInertiaFrame.current = 0;
    }

    // Two-finger scroll detection
    if (e.touches.length === 2) {
      releaseTap(); secondTap.current = false; isDragging.current = false;
      twoFingerRef.current = true;
      twoFingerStartTime.current = performance.now();
      twoFingerMoved.current = false;
      scrollPrimed.current = false;
      const midX = (e.touches[0].clientX + e.touches[1].clientX) / 2;
      const midY = (e.touches[0].clientY + e.touches[1].clientY) / 2;
      twoFingerLastX.current = midX;
      twoFingerLastY.current = midY;
      scrollVelocity.current = { x: 0, y: 0 };
      lastScrollTime.current = performance.now();
      // Do not let the remaining finger inherit stale single-finger movement
      // state when one finger is lifted at the end of a scroll.
      lastTouchRef.current = null;
      touchStartPos.current = null;
      return;
    }

    twoFingerRef.current = false;
    const touch = e.touches[0];
    gestureMoved.current = false;
    touchStartTime.current = performance.now();
    cursorVelocity.current = { x: 0, y: 0 };
    lastCursorTime.current = performance.now();

    // Check for double-tap-drag start. Only start a drag when the previous
    // gesture was a genuine tap AND this
    // touch lands close to where that gesture ended. This prevents a cursor move
    // followed by another quick move from being misread as a click-and-drag —
    // that sequence should just keep moving the cursor.
    const dist = lastTapEndPos.current
      ? Math.hypot(
          touch.clientX - lastTapEndPos.current.x,
          touch.clientY - lastTapEndPos.current.y,
        )
      : Infinity;

    secondTap.current = tapReleaseTimer.current !== null && tapMouseDown.current &&
      lastGestureAllowsDrag.current && touchStartTime.current - lastTouchEndTime.current < 300 && dist < 40;
    if (secondTap.current) {
      // Reuse the first press. Movement can now start a drag immediately without
      // generating the second native press that would maximize a titlebar.
      clearTimeout(tapReleaseTimer.current!);
      tapReleaseTimer.current = null;
    } else releaseTap();

    touchStartPos.current = { x: touch.clientX, y: touch.clientY };
    lastTouchRef.current = { x: touch.clientX, y: touch.clientY };
  };

  const onTouchMove = (e: React.TouchEvent) => {
    e.preventDefault();
    const now = performance.now();
    if (now - lastMoveTime.current < 16) return;
    lastMoveTime.current = now;

    // Two-finger scroll
    if (twoFingerRef.current) {
      // A two-finger gesture remains a scroll/right-click until every finger is
      // lifted. In particular, never reinterpret its last finger as cursor move.
      if (e.touches.length !== 2) return;
      const midX = (e.touches[0].clientX + e.touches[1].clientX) / 2;
      const midY = (e.touches[0].clientY + e.touches[1].clientY) / 2;
      // Finger placement can shift the initial midpoint before both contacts
      // settle. Start deltas at the first move, avoiding a spurious first jump.
      if (!scrollPrimed.current) {
        scrollPrimed.current = true;
        twoFingerMoved.current = Math.hypot(midX - twoFingerLastX.current, midY - twoFingerLastY.current) > 4;
        twoFingerLastX.current = midX;
        twoFingerLastY.current = midY;
        lastScrollTime.current = now;
        return;
      }
      const dx = (midX - twoFingerLastX.current) * settings.scrollSensitivity;
      const dy = (midY - twoFingerLastY.current) * settings.scrollSensitivity;
      twoFingerLastX.current = midX;
      twoFingerLastY.current = midY;

      const dt = now - lastScrollTime.current;
      if (dt > 0) {
        // Calculate velocity for inertia (pixels per ms)
        const vx = dx / dt;
        const vy = dy / dt;
        // Simple low-pass filter to smooth velocity
        const alpha = 0.5;
        scrollVelocity.current = {
          x: scrollVelocity.current.x * alpha + vx * (1 - alpha),
          y: scrollVelocity.current.y * alpha + vy * (1 - alpha),
        };
        lastScrollTime.current = now;
      }

      if (Math.abs(dx) > 0.5 || Math.abs(dy) > 0.5) {
        twoFingerMoved.current = true;
        send({
          type: "mousescroll",
          dx: Math.round(-dx),
          dy: Math.round(-dy),
          precise: true,
        });
      }
      return;
    }

    const touch = e.touches[0];
    if (touchStartPos.current && Math.hypot(touch.clientX - touchStartPos.current.x, touch.clientY - touchStartPos.current.y) >= 6) gestureMoved.current = true;
    if (lastTouchRef.current) {
      const rawDx = touch.clientX - lastTouchRef.current.x;
      const rawDy = touch.clientY - lastTouchRef.current.y;
      const dx = rawDx * settings.cursorSensitivity;
      const dy = rawDy * settings.cursorSensitivity;

      if (secondTap.current) {
        if (!gestureMoved.current) return;
        secondTap.current = false;
        isDragging.current = true;
      }
      // Stationary taps must not nudge the pointer out of the desktop's
      // double-click hit region. Begin cursor movement only after tap slop.
      if (!gestureMoved.current) return;
      send({ type: "mousemoverel", dx, dy });
      lastTouchRef.current = { x: touch.clientX, y: touch.clientY };

      const dt = now - lastCursorTime.current;
      if (dt > 0) {
        const vx = dx / dt;
        const vy = dy / dt;
        const alpha = 0.5;
        cursorVelocity.current = {
          x: cursorVelocity.current.x * alpha + vx * (1 - alpha),
          y: cursorVelocity.current.y * alpha + vy * (1 - alpha),
        };
        lastCursorTime.current = now;
      }
    }
  };

  const onTouchEnd = (e: React.TouchEvent) => {
    e.preventDefault();

    // Two-finger scroll end
    if (twoFingerRef.current) {
      if (e.touches.length === 0) {
        twoFingerRef.current = false;

        const { x: vx, y: vy } = scrollVelocity.current;
        const timeSinceLastScroll = performance.now() - lastScrollTime.current;

        if (
          !twoFingerMoved.current &&
          performance.now() - twoFingerStartTime.current < 300
        ) {
          send({ type: "mouseclick", btn: MouseButton.RIGHT, modifiers: activeModifiers });
          // Only trigger inertia if the user was scrolling recently and with sufficient velocity
        } else if (
          timeSinceLastScroll < 50 &&
          (Math.abs(vx) > 0.1 || Math.abs(vy) > 0.1)
        ) {
          let cx = vx;
          let cy = vy;
          let lastT = performance.now();

          const step = () => {
            const t = performance.now();
            const dt = Math.min(32, t - lastT);
            lastT = t;

            // Apply friction to velocity
            const friction = Math.pow(settings.scrollFriction, dt / 16);
            cx *= friction;
            cy *= friction;

            // Stop when velocity is negligible
            if (Math.abs(cx) < 0.05 && Math.abs(cy) < 0.05) {
              inertiaFrame.current = 0;
              return;
            }

            const dx = cx * dt;
            const dy = cy * dt;
            send({
              type: "mousescroll",
              dx: Math.round(-dx),
              dy: Math.round(-dy),
              precise: true,
            });
            inertiaFrame.current = requestAnimationFrame(step);
          };
          inertiaFrame.current = requestAnimationFrame(step);
        }
      }
      return;
    }

    const now = performance.now();
    lastTouchEndTime.current = now;

    if (secondTap.current) {
      secondTap.current = false;
      releaseTap();
      if (!gestureMoved.current && now - touchStartTime.current < 200) {
        // Give the two complete native clicks a short, real interval instead
        // of emitting a zero-duration burst that some desktops ignore.
        doubleClickTimer.current = setTimeout(() => {
          doubleClickTimer.current = null;
          send({ type: "mouseclick", btn: MouseButton.LEFT, modifiers: activeModifiers });
        }, 40);
      }
      lastGestureAllowsDrag.current = false;
      lastTapEndPos.current = { x: e.changedTouches[0].clientX, y: e.changedTouches[0].clientY };
      touchStartPos.current = null; lastTouchRef.current = null;
      return;
    }
    if (isDragging.current) {
      releaseTap();
      isDragging.current = false;
      lastGestureAllowsDrag.current = false;
    } else {
      // Cursor inertia
      const { x: vx, y: vy } = cursorVelocity.current;
      const timeSinceLastMove = performance.now() - lastCursorTime.current;

      if (
        timeSinceLastMove < 50 &&
        (Math.abs(vx) > 0.1 || Math.abs(vy) > 0.1)
      ) {
        let cx = vx;
        let cy = vy;
        let lastT = performance.now();

        const step = () => {
          const t = performance.now();
          const dt = t - lastT;
          lastT = t;

          const friction = Math.pow(settings.cursorFriction, dt / 16);
          cx *= friction;
          cy *= friction;

          if (Math.abs(cx) < 0.05 && Math.abs(cy) < 0.05) {
            cursorInertiaFrame.current = 0;
            return;
          }

          const dx = cx * dt;
          const dy = cy * dt;
          send({ type: "mousemoverel", dx, dy });
          cursorInertiaFrame.current = requestAnimationFrame(step);
        };
        cursorInertiaFrame.current = requestAnimationFrame(step);
      }

      // A genuine tap (short and near-stationary) is a click, and it also arms
      // the next quick touch to become a click-and-drag. A longer or moved
      // gesture was a cursor move, which must not arm a drag.
      let wasTap = false;
      if (!gestureMoved.current && touchStartPos.current && now - touchStartTime.current < 200) {
        const touch = e.changedTouches[0];
        const dx = touch.clientX - touchStartPos.current.x;
        const dy = touch.clientY - touchStartPos.current.y;
        if (Math.sqrt(dx * dx + dy * dy) < 6) {
          send({ type: "mousedown", btn: MouseButton.LEFT, modifiers: activeModifiers });
          tapMouseDown.current = true;
          // Press feedback is immediate; only release waits for a very quick
          // retouch, which can continue the same press as a drag.
          tapReleaseTimer.current = setTimeout(() => {
            tapReleaseTimer.current = null;
            if (tapMouseDown.current) send({ type: "mouseup", btn: MouseButton.LEFT, modifiers: activeModifiers });
            tapMouseDown.current = false;
          }, 100);
          wasTap = true;
        }
      }
      lastGestureAllowsDrag.current = wasTap;
    }

    // Remember where this gesture lifted so the next touch can measure closeness.
    const endTouch = e.changedTouches[0];
    if (endTouch)
      lastTapEndPos.current = { x: endTouch.clientX, y: endTouch.clientY };

    touchStartPos.current = null;
    lastTouchRef.current = null;
  };

  // Mouse drag via Pointer Lock (or fallback position tracking after Escape cooldown)
  const onPointerDown = (e: React.PointerEvent) => {
    activateKeyboard();
    dismissDraft();
    if (e.pointerType === "touch") return;
    setDraftVisible(false);

    const modifiers = combineModifiers(getModifiers(e), activeModifiers);

    // Middle click
    if (e.button === 1) {
      e.preventDefault();
      send({ type: "mouseclick", btn: MouseButton.MIDDLE, modifiers });
      return;
    }

    // Right click handled by onContextMenu
    if (e.button === 2) {
      e.preventDefault();
      send({ type: "mouseclick", btn: MouseButton.RIGHT, modifiers }); // TODO: Implement MIDDLE/BACK/FORWARD
      return;
    }

    // Left button: grab pointer lock if not already locked
    if (!document.pointerLockElement) {
      // requestPointerLock may fail after Escape (browser cooldown ~1.5s).
      // In that case, fallback mouse tracking via lastMousePos kicks in.
      void trackpadRef.current?.requestPointerLock();
      lastMousePos.current = { x: e.clientX, y: e.clientY };
    }
    if (settings.customKeyboard) trackpadRef.current?.focus();
    else inputRef.current?.focus();

    // Handle click/drag
    if (document.pointerLockElement && settings.clickAndDrag) {
      if (dragLocked.current) {
        send({ type: "mouseup", btn: MouseButton.LEFT, modifiers }); // TODO: Left or right or other?
        dragLocked.current = false;
      } else {
        send({ type: "mousedown", btn: MouseButton.LEFT, modifiers });
        dragLocked.current = true;
      }
    } else {
      mouseHeld.current = true;
      send({ type: "mousedown", btn: MouseButton.LEFT, modifiers });
    }
  };

  // Mouse movement and release (pointer lock + fallback)
  useEffect(() => {
    if (!connected) return;

    const handleMouseMove = (e: MouseEvent) => {
      const now = performance.now();
      if (now - lastMoveTime.current < 16) return;
      lastMoveTime.current = now;

      if (document.pointerLockElement) {
        // Pointer lock active: use movementX/Y
        send({
          type: "mousemoverel",
          dx: e.movementX * settings.cursorSensitivity,
          dy: e.movementY * settings.cursorSensitivity,
        });
      } else if (mouseHeld.current && lastMousePos.current) {
        // Fallback: track delta manually (after Escape cooldown denies pointer lock)
        const dx =
          (e.clientX - lastMousePos.current.x) * settings.cursorSensitivity;
        const dy =
          (e.clientY - lastMousePos.current.y) * settings.cursorSensitivity;
        lastMousePos.current = { x: e.clientX, y: e.clientY };
        if (dx !== 0 || dy !== 0) {
          send({ type: "mousemoverel", dx, dy });
        }
      }
    };

    const handleMouseUp = (e: MouseEvent) => {
      if (mouseHeld.current) {
        mouseHeld.current = false;
        lastMousePos.current = null;
        send({
          type: "mouseup",
          btn: MouseButton.LEFT,
          modifiers: combineModifiers(getModifiers(e), activeModifiers),
        }); // TODO: Handle non-LEFT
      }
    };

    const handlePointerLockChange = () => {
      if (!document.pointerLockElement) {
        if (mouseHeld.current) {
          mouseHeld.current = false;
          send({ type: "mouseup", btn: MouseButton.LEFT, modifiers: activeModifiers });
        }
        if (dragLocked.current) {
          dragLocked.current = false;
          send({ type: "mouseup", btn: MouseButton.LEFT, modifiers: activeModifiers });
        }
        lastMousePos.current = null;
        // Don't blur - keep trackpad focused for keyboard events and easy re-lock
      }
    };

    document.addEventListener("mousemove", handleMouseMove);
    document.addEventListener("mouseup", handleMouseUp);
    document.addEventListener("pointerlockchange", handlePointerLockChange);
    return () => {
      document.removeEventListener("mousemove", handleMouseMove);
      document.removeEventListener("mouseup", handleMouseUp);
      document.removeEventListener(
        "pointerlockchange",
        handlePointerLockChange,
      );
      if (document.pointerLockElement) document.exitPointerLock();
    };
  }, [activeModifiers, connected, send, settings.cursorSensitivity]);

  // Keyboard capture on the trackpad div
  useEffect(() => {
    const el = trackpadRef.current;
    if (!el || !connected) return;

    el.addEventListener("keydown", handleKeyDown);
    el.addEventListener("keyup", handleKeyUp);
    return () => {
      el.removeEventListener("keydown", handleKeyDown);
      el.removeEventListener("keyup", handleKeyUp);
    };
  }, [connected, handleKeyDown, handleKeyUp]);

  // Wheel scroll handler (passive: false to allow preventDefault)
  useEffect(() => {
    const el = trackpadRef.current;
    if (!el || !connected) return;

    const handleWheel = (e: WheelEvent) => {
      dismissDraft();
      e.preventDefault();
      // deltaMode: 0 = pixels (trackpads), 1 = lines (mouse wheels), 2 = pages
      if (e.deltaMode === 1) {
        // Line-based scrolling (mouse wheel)
        const dx = Math.round(e.deltaX);
        const dy = Math.round(e.deltaY * settings.scrollSensitivity);
        if (dx !== 0 || dy !== 0) {
          send({ type: "mousescroll", dx, dy });
        }
      } else if (e.deltaMode === 2) {
        // Page-based: treat as lines with a multiplier
        const dx = Math.round(e.deltaX * 10);
        const dy = Math.round(e.deltaY * 10 * settings.scrollSensitivity);
        if (dx !== 0 || dy !== 0) {
          send({ type: "mousescroll", dx, dy });
        }
      } else {
        // Pixel-based (trackpads, smooth scrolling)
        const dx = Math.round(e.deltaX);
        const dy = Math.round(e.deltaY);
        if (Math.abs(dx) > 0.5 || Math.abs(dy) > 0.5) {
          send({ type: "mousescroll", dx, dy, precise: true });
        }
      }
    };

    el.addEventListener("wheel", handleWheel, { passive: false });
    return () => el.removeEventListener("wheel", handleWheel);
  }, [connected, dismissDraft, send, settings.scrollSensitivity]);


  const preview = textPreview(remote, draft);
  const previewCursor = remote?.password ? preview.length : remote?.available ? utf16Offset(preview, remote.selectionStart - remote.windowStart) : draftSelection ? utf16Offset(preview, Math.min(...draftSelection)) : draftCursor;
  const previewSelectionEnd = remote?.password ? preview.length : remote?.available ? utf16Offset(preview, remote.selectionEnd - remote.windowStart) : draftSelection ? utf16Offset(preview, Math.max(...draftSelection)) : draftCursor;

  const hasPreview = !!(preview || remote?.available || remote?.password) && draftVisible;
  const previewStatus = <>
    {!settings.mirrorFocusedText && <button type="button" aria-label="Enable focused text mirroring" onClick={() => onSettingsChange({ mirrorFocusedText: true })} className="pointer-events-auto rounded-md bg-zinc-950/80 px-2 py-1 text-[10px] text-blue-400">Enable preview</button>}
    <span role="status" aria-label={settings.mirrorFocusedText ? textMirroringStatus(connected, remote) : "Desktop text: mirroring is off"}
      title={settings.mirrorFocusedText ? textMirroringStatus(connected, remote) : "Desktop text: mirroring is off"}
      data-native-text-status className="pointer-events-auto flex h-7 w-5 shrink-0 items-center justify-center">
      <span aria-hidden="true" className={`h-1.5 w-1.5 rounded-full ${!settings.mirrorFocusedText ? "bg-zinc-500" : remote?.password || remote?.protected ? "bg-amber-400" : remote?.available ? "bg-emerald-400" : "bg-zinc-500"}`} />
    </span>
  </>;

  return (
    <div className={docked ? "remote-input flex min-h-0 flex-1 flex-col gap-2" : "w-full md:max-w-sm md:shrink-0 flex flex-col gap-2"}>
      <div className={docked ? "flex min-h-28 flex-1 gap-2" : "contents"}>
      {sideControls}
      <div className={docked ? "relative min-w-0 flex-1" : "relative"}>
        <div
          ref={trackpadRef}
          tabIndex={0}
          aria-label="Trackpad. Tap to click, move one finger to point, tap two fingers to right click, or move two fingers to scroll."
          className={`relative w-full ${docked ? "h-full" : "aspect-square max-h-[60vh]"} overflow-hidden rounded-xl border-2 transition-colors select-none touch-none outline-none ${
            connected
              ? focused || keyboardOpen
                ? "border-blue-500 ring-2 ring-blue-500/30 bg-zinc-900/80 cursor-crosshair"
                : "border-zinc-700 bg-zinc-900/80 cursor-crosshair"
              : "border-red-500/50 bg-zinc-900/40 pointer-events-none opacity-50"
          }`}
          style={
            connected
              ? {
                  backgroundImage:
                    "radial-gradient(circle, rgba(63,63,70,0.3) 1px, transparent 1px)",
                  backgroundSize: "20px 20px",
                  maxHeight: !docked && keyboardOpen ? `max(160px, calc(100dvh - ${settings.tallerKeys ? 428 : 380}px))` : undefined,
                }
              : undefined
          }
          onFocus={() => {
            setFocused(true);
            if (settings.customKeyboard) setKeyboardOpen(true);
            else queueMicrotask(() => inputRef.current?.focus());
          }}
          onBlur={() => setFocused(false)}
          onTouchStart={connected ? onTouchStart : undefined}
          onTouchMove={connected ? onTouchMove : undefined}
          onTouchEnd={connected ? onTouchEnd : undefined}
          onTouchCancel={() => {
            releaseTap(); secondTap.current = false;
            if (doubleClickTimer.current) clearTimeout(doubleClickTimer.current);
            doubleClickTimer.current = null;
            isDragging.current = false;
            lastGestureAllowsDrag.current = false;
            lastTouchRef.current = null;
          }}
          onPointerDown={connected ? onPointerDown : undefined}
          onContextMenu={(e) => {
            e.preventDefault();
          }}
        >
          {!connected && (
            <div className="flex flex-col items-center justify-center h-full text-zinc-500 text-sm gap-2">
              {connecting ? (
                <>
                  <div className="w-5 h-5 border-2 border-zinc-600 border-t-zinc-400 rounded-full animate-spin" />
                  <span>Connecting...</span>
                </>
              ) : (
                <span>Disconnected</span>
              )}
            </div>
          )}
        </div>
        {connected && !hasPreview && <div className="absolute bottom-3 right-16 z-20 flex items-center gap-1">{previewStatus}</div>}
        {connected && hasPreview && (
          <div className="pointer-events-none absolute inset-3 z-20 flex flex-col justify-end gap-1 pr-11" data-text-preview>
            <div className="flex items-center justify-end gap-1">
              {previewStatus}
              <button type="button" aria-label="Clear preview" title="Clear preview" onClick={dismissDraft}
                className="pointer-events-auto flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-white/10 bg-zinc-950/80 text-zinc-400 hover:text-white"><Eraser className="h-3.5 w-3.5" /></button>
            </div>
            <div data-preview-scroll data-preview-interactive={settings.interactivePreview} className={`${settings.interactivePreview ? "pointer-events-auto" : "pointer-events-none"} min-h-0 overflow-y-auto overscroll-contain rounded-lg border border-white/5 bg-zinc-950/65 shadow-lg backdrop-blur-md`}>
              <div ref={previewRef} className="relative shrink-0 touch-pan-y select-none" aria-live="polite" aria-atomic="true"
                onPointerDown={(event) => { touchStartPos.current = { x: event.clientX, y: event.clientY }; }}
                onPointerUp={(event) => {
                  const start = touchStartPos.current;
                  touchStartPos.current = null;
                  if (!start || Math.hypot(event.clientX - start.x, event.clientY - start.y) > 8) return;
                  const offset = previewHit(event.currentTarget, event.clientX, event.clientY);
                  if (offset !== undefined) { navigationX.current = undefined; selectPreview(offset); showDraftIndicator(); }
                }}
                aria-label={remote?.password ? "Password field. Contents hidden." : undefined}
                data-password-preview={remote?.password || undefined}>
                <CaretText value={preview} cursor={previewCursor} selectionEnd={previewSelectionEnd} hideCaret={!!remote?.password}
                  dragging={cursorDrag !== null}
                  variant="preview" className="shrink-0 whitespace-pre-wrap break-words px-3 py-2 text-sm leading-5 text-zinc-200 [overflow-wrap:anywhere]" />
                {cursorDrag && <span aria-hidden="true" data-drag-caret className="pointer-events-none absolute left-0 top-0 z-30 w-0.5 rounded-full bg-blue-400 shadow-[0_0_8px_rgba(96,165,250,.5)]"
                  style={{ height: cursorDrag.height, transform: `translate3d(${cursorDrag.left}px,${cursorDrag.top}px,0)` }} />}
              </div>
            </div>
          </div>
        )}
        {connected && (
          <ScrollRail send={send} sensitivity={settings.scrollSensitivity} friction={settings.scrollFriction} haptics={settings.haptics} onInteract={dismissDraft} />
        )}
        {!settings.customKeyboard && (
          <button
            type="button"
            disabled={!connected}
            aria-label="Open keyboard settings"
            title="Keyboard settings"
            onClick={() => {
              setKeyboardSettingsOpen(true);
              setKeyboardOpen(true);
            }}
            className="absolute bottom-4 left-16 z-20 flex h-8 w-8 items-center justify-center rounded-full border border-white/10 bg-zinc-950/65 text-zinc-400 shadow-lg backdrop-blur-md transition-colors hover:bg-zinc-800 hover:text-white disabled:opacity-40"
          >
            <Settings className="h-3.5 w-3.5" aria-hidden="true" />
          </button>
        )}
        {!settings.customKeyboard && (
          <textarea
            ref={inputRef}
            rows={1}
            value={compositionValue ?? (remote?.available && remote.editable ? remote.value : draft)}
            disabled={!connected}
            aria-label="Type on the remote computer with the device keyboard"
            className="pointer-events-none absolute bottom-2 left-2 h-px w-px resize-none opacity-0"
            autoComplete={remote?.password ? "off" : "on"}
            autoCapitalize={remote?.password ? "none" : "sentences"}
            autoCorrect={remote?.password ? "off" : "on"}
            spellCheck={!remote?.password}
            enterKeyHint="enter"
            inputMode="text"
            onFocus={() => setFocused(true)}
            onBlur={() => setFocused(false)}
            onKeyDown={handleInputKeyDown}
            onCompositionStart={(e) => {
              composing.current = true;
              const current = remoteModel.current!.context;
              compositionContext.current = current ? {fieldId: current.fieldId, revision: current.revision} : null;
              setCompositionValue(e.currentTarget.value);
            }}
            onCompositionEnd={(e) => {
              composing.current = false;
              setCompositionValue(null);
              const initial = compositionContext.current;
              const current = remoteModel.current!.context;
              if (initial?.fieldId !== current?.fieldId || initial?.revision !== current?.revision) return;
              handleTextChange(e as unknown as React.ChangeEvent<HTMLTextAreaElement>);
            }}
            onChange={handleTextChange}
            onSelect={handleSelection}
          />
        )}
      </div>
      </div>
      <div className={docked ? "shrink-0" : "relative"}>
        <CustomKeyboard
          docked={docked}
          key={keyboardSettingsOpen ? "settings" : "keyboard"}
          open={docked || keyboardOpen}
          connected={connected}
          remoteOS={remoteOS}
          textContext={typingContext}
          textBeforeCursor={draftVisible && !remote?.protected && !remote?.password && previewCursor === previewSelectionEnd ? preview.slice(0, previewCursor) : ""}
          sensitiveInput={!!(remote?.protected || remote?.password)}
          settings={settings}
          onText={appendText}
          onReplaceText={(previous, next, suggested = false) => {
            const view = remoteModel.current!.view;
            const position = view?.available ? view.selectionStart : remoteCursor.current;
            const value = view?.available ? view.value : draftRef.current;
            const beforeCaret = Array.from(value).slice(0, position - (view?.available ? view.windowStart : 0)).join("");
            if (!beforeCaret.endsWith(previous) || (view?.available && view.selectionStart !== view.selectionEnd)) return;
            if (!replaceRemote(position - characterCount(previous), position, next)) {
              if (!view?.available) editFallback(position - characterCount(previous), position, next);
              else { for (let i = 0; i < characterCount(previous); i++) sendKeyPress("Backspace"); send({ type: "text", text: next }); updateNativePreview(position - characterCount(previous), position, next); }
            }
            typingMemory.current.field = view?.fieldId;
            typingMemory.current.space = suggested ? position - characterCount(previous) + characterCount(next) : null;
            typingMemory.current.suppress = false;
            consumeModifiers();
          }}
          onBackspace={backspace}
          onEnter={(shifted) => {
            // Enter is an action, even in multiline fields: the remote app
            // decides whether it submits or inserts a line break.
            const modifiers = combineModifiers(
              activeModifiers,
              shifted ? [Modifier.SHIFT] : [],
            );
            sendKeyPress("Enter", modifiers);
            dismissDraft();
          }}
          onTab={() => { dismissDraft(); sendKeyPress("Tab", activeModifiers); }}
          onPasteImage={() => {
            dismissDraft();
            modifierState.current = new KeyboardModifiers(); syncModifiers();
            const modifier = remoteOS === "darwin" ? Modifier.META : Modifier.CTRL;
            send({ type: "keydown", key: "v", modifiers: [modifier] });
            send({ type: "keyup", key: "v", modifiers: [modifier] });
            send({ type: "keyup", key: remoteOS === "darwin" ? "Meta" : "Control", modifiers: [] });
          }}
          onNavigate={navigate}
          onCursorDrag={handleCursorDrag}
          onClose={() => {
            dragOrigin.current = null; setCursorDrag(null);
            modifierState.current = new KeyboardModifiers(); syncModifiers();
            setKeyboardOpen(false);
            setKeyboardSettingsOpen(false);
          }}
          onSettingsChange={onSettingsChange}
          onUseCustomKeyboard={() => onSettingsChange({ customKeyboard: true })}
          onUseDeviceKeyboard={() => {
            dragOrigin.current = null; setCursorDrag(null);
            onSettingsChange({ customKeyboard: false });
            setKeyboardOpen(false);
            setKeyboardSettingsOpen(false);
            requestAnimationFrame(() => inputRef.current?.focus());
          }}
          usingCustomKeyboard={settings.customKeyboard}
          settingsPageOpen={keyboardSettingsOpen}
          modifiers={activeModifiers}
          lockedModifiers={lockedModifiers}
          onModifierDown={beginModifier}
          onModifierUp={endModifier}
          onSuperPress={() => { dismissDraft(); sendKeyPress("Meta"); }}
        />
      </div>
      {keyboardOpen && !docked && (
        <div className={settings.tallerKeys ? "h-[408px] md:hidden" : "h-[360px] md:hidden"} aria-hidden="true" />
      )}
    </div>
  );
}

export function Trackpad({
  mode = "dashboard",
  settingsContainer,
  sideControls,
  textContext,
  connected,
  connecting,
  cursorPos,
  cursorSupported,
  send,
}: {
  textContext: TrackpadMessageTextContext | null;
  connected: boolean;
  connecting: boolean;
  cursorPos: { x: number; y: number } | null;
  cursorSupported: boolean;
  mode?: "dashboard" | "remote";
  settingsContainer?: HTMLElement | null;
  sideControls?: React.ReactNode;
  send: (msg: TrackpadMessage) => void;
}) {
  const layouts = useStore((s) => s.layouts);
  const currentLayout = useStore((s) => s.currentLayout);
  const loading = useStore((s) => s.layoutsLoading);
  const [showSettings, setShowSettings] = useState(false);
  const [settings, setSettings] = useState<TrackpadSettings>(loadSettings);
  const settingsRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    savePreference("ottoman-trackpad-settings", settings);
  }, [settings]);

  useEffect(() => {
    if (showSettings) {
      const onClick = (e: MouseEvent) => {
        if (
          settingsRef.current &&
          !settingsRef.current.contains(e.target as Node)
        ) {
          setShowSettings(false);
        }
      };
      document.addEventListener("mousedown", onClick);
      return () => document.removeEventListener("mousedown", onClick);
    }
  }, [showSettings]);

  const controls = (
      <div className={`flex items-center justify-between ${mode === "remote" ? "" : "mb-4"}`}>
        {mode !== "remote" && <h2 className="text-lg font-semibold text-zinc-200 flex items-center gap-2">
          {"Trackpad"}
          <span
            className={`inline-block w-2 h-2 rounded-full ${
              connected ? "bg-green-400" : "bg-red-400"
            }`}
          />
        </h2>}
        <div className="relative" ref={settingsRef}>
          <button
            onClick={() => setShowSettings(!showSettings)}
            title={showSettings ? "Close settings" : "Trackpad settings"}
            aria-label={showSettings ? "Close settings" : "Trackpad settings"}
            className={`w-8 h-8 flex items-center justify-center rounded-md transition-colors border cursor-pointer ${
              showSettings
                ? "bg-zinc-700 border-zinc-600 text-zinc-100"
                : "bg-zinc-800 hover:bg-zinc-700 border-zinc-700 text-zinc-300"
            }`}
          >
            {showSettings ? (
              <X className="w-4 h-4" />
            ) : (
              <Settings className="w-4 h-4" />
            )}
          </button>
          {showSettings && (
            <div className="absolute right-0 top-full z-[60] mt-2 flex max-h-[70vh] w-72 flex-col gap-4 overflow-y-auto rounded-lg border border-zinc-700 bg-zinc-900 p-4 shadow-xl">
              <div className="text-[10px] font-semibold uppercase tracking-[0.14em] text-zinc-500">
                Trackpad
              </div>
              {TRACKPAD_RANGES.map(({ key, label, min, max, step, decimals }, index) => (
                <div key={key}>
                  {index === 2 && <div className="mb-4 h-px bg-zinc-800" />}
                  <label className="block text-xs text-zinc-400 mb-1">
                    {label} ({settings[key].toFixed(decimals)})
                    <input
                      type="range"
                      min={min}
                      max={max}
                      step={step}
                      value={settings[key]}
                      onChange={(event) => {
                        const value = Number(event.target.value);
                        setSettings((current) => ({ ...current, [key]: value }));
                      }}
                      className="w-full accent-blue-500"
                    />
                  </label>
                </div>
              ))}
              <div className="h-px bg-zinc-800" />
              <div className="flex items-center justify-between">
                <label className="text-xs text-zinc-400">Click and Drag</label>
                <input
                  type="checkbox"
                  checked={settings.clickAndDrag}
                  onChange={(e) =>
                    setSettings({ ...settings, clickAndDrag: e.target.checked })
                  }
                />
              </div>
            </div>
          )}
        </div>
      </div>
  );

  return (
    <section className={mode === "remote" ? "flex min-h-0 flex-1 flex-col" : undefined}>
      {mode === "remote" ? settingsContainer && createPortal(controls, settingsContainer) : controls}
      <div className={mode === "remote" ? "flex min-h-0 flex-1 flex-col" : "flex flex-col-reverse md:flex-row gap-6 sm:items-start"}>
        <TouchArea
          key={connected ? "connected" : "disconnected"}
          sideControls={sideControls}
          docked={mode === "remote"}
          textContext={textContext}
          connected={connected}
          connecting={connecting}
          send={send}
          settings={settings}
          onSettingsChange={(patch) => setSettings((current) => ({ ...current, ...patch }))}
        />
        {mode !== "remote" && <MonitorDisplay
          layouts={layouts}
          currentLayout={currentLayout}
          cursorPos={cursorPos}
          connected={connected}
          loading={loading}
          onSetPosition={
            cursorSupported
              ? (x, y) => send({ type: "mousemoveto", x, y })
              : undefined
          }
        />}
      </div>
    </section>
  );
}
