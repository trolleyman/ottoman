import { useEffect, useRef, useState } from "react";
import { haptic } from "./keyboardHaptics";
import { Modifier } from "./api";
import type { KeyboardSettings } from "./trackpadSettings";
import type { KeyDefinition } from "./keyboardData";

export function ActionKey({ onPress, onHold, holdDelayMs = 400, preview, children, className = "", ...props }: Omit<React.ButtonHTMLAttributes<HTMLButtonElement>,
  "onClick" | "onPointerDown" | "onPointerUp" | "onPointerCancel" | "onLostPointerCapture"> & { onPress: () => void; onHold?: () => void; holdDelayMs?: number; preview?: React.ReactNode }) {
  const pointer = useRef<number | null>(null);
  const [pressed, setPressed] = useState(false);
  const [held, setHeld] = useState(false);
  const didHold = useRef(false);
  const holdTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => () => { if (holdTimer.current) clearTimeout(holdTimer.current); }, []);
  const cancel = () => {
    if (holdTimer.current) clearTimeout(holdTimer.current);
    holdTimer.current = null;
    pointer.current = null;
    setPressed(false);
    setHeld(false);
  };
  return <button {...props} className={`${className} relative ${pressed ? "z-50" : ""}`}
    onPointerDown={(event) => {
      if (event.button !== 0 || pointer.current !== null) return;
      event.preventDefault();
      didHold.current = false;
      pointer.current = event.pointerId;
      setPressed(true);
      event.currentTarget.setPointerCapture(event.pointerId);
      if (onHold) holdTimer.current = setTimeout(() => {
        didHold.current = true;
        setHeld(true);
        onHold();
      }, holdDelayMs);
    }}
    onPointerUp={(event) => {
      if (pointer.current !== event.pointerId) return;
      cancel();
      if (!didHold.current) onPress();
    }}
    onPointerCancel={cancel}
    onLostPointerCapture={cancel}
    onClick={(event) => { if (event.detail === 0) onPress(); }}
  >{children}{pressed && preview && (!onHold || held) && <KeyPreview>{preview}</KeyPreview>}</button>;
}

export function KeyPreview({ children }: { children: React.ReactNode }) {
  return <span aria-hidden="true" data-key-preview="action" className="key-press-preview pointer-events-none absolute bottom-full left-1/2 z-50 mb-1.5 flex h-12 min-w-12 -translate-x-1/2 items-center justify-center rounded-xl border border-zinc-400 bg-zinc-500 px-2 text-sm font-medium text-white shadow-xl">{children}</span>;
}

export function WindowsIcon({ className = "" }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" className={className}>
      <path d="M3 5.4 11 4.3v7.2H3V5.4Zm9.2-1.3L21 2.9v8.6h-8.8V4.1ZM3 12.7h8v7.2l-8-1.1v-6.1Zm9.2 0H21v8.6l-8.8-1.2v-7.4Z" />
    </svg>
  );
}

export function ModifierButton({
  modifier,
  label,
  text,
  selected,
  onToggle,
  onHold,
  holdDelayMs,
  children,
}: {
  modifier: Modifier;
  label: string;
  text: string;
  selected: Modifier[];
  onToggle: (modifier: Modifier) => void;
  onHold?: () => void;
  holdDelayMs?: number;
  children: React.ReactNode;
}) {
  const active = selected.includes(modifier);
  return (
    <ActionKey
      type="button"
      aria-label={label}
      title={label}
      aria-pressed={active}
      onPress={() => onToggle(modifier)}
      onHold={onHold}
      holdDelayMs={holdDelayMs}
      preview={onHold ? <span className="flex items-center gap-2">{children}{text}</span> : undefined}
      className={`flex h-9 min-w-10 flex-1 items-center justify-center gap-2 rounded-md border px-1 transition-colors ${
        active
          ? "border-blue-400/60 bg-blue-500 text-white shadow-[0_0_0_2px_rgba(59,130,246,.16)]"
          : "custom-keyboard-utility border-zinc-800 bg-zinc-900 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100"
      }`}
    >
      {children}
      <span className="text-[10px] font-semibold tracking-wide">{text}</span>
    </ActionKey>
  );
}

export function Key({
  definition,
  label,
  settings,
  onType,
  swipeActive = false,
  suppressTap,
}: {
  definition: KeyDefinition;
  label?: string;
  settings: KeyboardSettings;
  onType: (value: string, wasLongPress: boolean) => void;
  swipeActive?: boolean;
  suppressTap?: (pointerId: number) => boolean;
}) {
  // Pointer ownership is synchronous. React state is only for painting the
  // preview: a fast down/up pair can complete before the next render.
  const pointers = useRef(new Map<number, { held: boolean; timer: ReturnType<typeof setTimeout> | null }>());
  const [pressed, setPressed] = useState(false);
  const [alternate, setAlternate] = useState(false);
  useEffect(() => {
    const active = pointers.current;
    return () => { for (const pointer of active.values()) if (pointer.timer) clearTimeout(pointer.timer); };
  }, []);
  const cancel = (id: number) => {
    const pointer = pointers.current.get(id);
    if (pointer?.timer) clearTimeout(pointer.timer);
    pointers.current.delete(id);
    setPressed(pointers.current.size > 0);
    setAlternate(false);
  };
  const start = (event: React.PointerEvent<HTMLButtonElement>) => {
    if (event.button !== 0) return;
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    const pointer = { held: false, timer: null as ReturnType<typeof setTimeout> | null };
    pointers.current.set(event.pointerId, pointer);
    setPressed(true);
    setAlternate(false);
    haptic(settings.haptics);
    if (!definition.alt) return;
    pointer.timer = setTimeout(() => {
      pointer.held = true;
      setAlternate(true);
      haptic(settings.haptics, 16);
    }, settings.longPressDelayMs);
  };
  useEffect(() => {
    if (swipeActive) for (const pointer of pointers.current.values()) {
      if (pointer.timer) clearTimeout(pointer.timer);
    }
  }, [swipeActive]);
  const finish = (event: React.PointerEvent<HTMLButtonElement>) => {
    const pointer = pointers.current.get(event.pointerId);
    if (!pointer) return;
    if (!suppressTap?.(event.pointerId)) {
      onType(pointer.held && definition.alt ? definition.alt : definition.key, pointer.held);
    }
    cancel(event.pointerId);
  };
  const displayed = alternate ? definition.alt : label ?? definition.key;

  return (
    <button
      type="button"
      data-letter={/^[a-z]$/.test(definition.key) ? definition.key : undefined}
      className={`custom-keyboard-key relative flex min-w-0 flex-1 touch-none select-none items-center justify-center border-0 bg-transparent p-0 text-[clamp(15px,4.5vw,20px)] font-medium text-white ${pressed && !swipeActive ? "z-30" : ""}`}
      style={definition.grow ? { flexGrow: definition.grow } : undefined}
      aria-label={definition.alt ? `${definition.key}, hold for ${definition.alt}` : definition.key}
      onPointerDown={start}
      onPointerUp={finish}
      onPointerCancel={(event) => cancel(event.pointerId)}
      onLostPointerCapture={(event) => cancel(event.pointerId)}
      onClick={(event) => {
        // Assistive activation has no preceding pointer events.
        if (event.detail === 0) {
          haptic(settings.haptics);
          onType(definition.key, false);
        }
      }}
      onContextMenu={(event) => event.preventDefault()}
    >
      <span aria-hidden="true" className={`key-face absolute inset-[2px] rounded-md border border-white/10 bg-zinc-600/95 shadow-[0_1px_0_rgba(0,0,0,.7),0_2px_3px_rgba(0,0,0,.35)] ${pressed && !swipeActive ? "ring-2 ring-blue-400/60" : ""}`} />
      <span className="relative">{label ?? definition.key}</span>
      {settings.symbolHints && definition.alt && (
        <span className="custom-keyboard-hint absolute right-1.5 top-1 text-[9px] font-semibold text-zinc-300/55">
          {definition.alt}
        </span>
      )}
      {pressed && !swipeActive && (
        <span aria-hidden="true" data-key-preview={alternate ? "alternate" : "letter"}
          className={`key-press-preview pointer-events-none absolute -top-14 left-1/2 z-40 flex h-14 w-12 -translate-x-1/2 items-center justify-center rounded-xl border text-3xl shadow-xl ${alternate ? "border-blue-300 bg-blue-500 text-white" : "border-zinc-400 bg-zinc-500 text-white"}`}>
          {displayed}
          <span className="absolute -bottom-1.5 h-3 w-3 rotate-45 rounded-sm bg-inherit" />
        </span>
      )}
    </button>
  );
}

export function UtilityKey({
  children,
  label,
  active = false,
  wide = false,
  onPress,
}: {
  children: React.ReactNode;
  label: string;
  active?: boolean;
  wide?: boolean;
  onPress: () => void;
}) {
  return (
    <ActionKey
      type="button"
      aria-label={label}
      preview={children}
      onPress={onPress}
      className={`keyboard-cell flex touch-none select-none items-center justify-center rounded-md border text-sm font-semibold shadow-[0_1px_0_rgba(0,0,0,.7),0_2px_3px_rgba(0,0,0,.3)] active:translate-y-px ${
        wide ? "min-w-[52px] flex-[1.4]" : "min-w-0 flex-1"
      } ${
        active
          ? "border-blue-400/50 bg-blue-500 text-white"
          : "custom-keyboard-utility border-white/5 bg-zinc-800 text-zinc-100 active:bg-zinc-700"
      }`}
    >
      {children}
    </ActionKey>
  );
}

export function NavigationKey({
  label,
  children,
  onPress,
}: {
  label: string;
  children: React.ReactNode;
  onPress: () => void;
}) {
  return (
    <ActionKey
      type="button"
      aria-label={label}
      preview={children}
      onPress={onPress}
      className="custom-keyboard-utility flex h-8 min-w-0 flex-1 touch-none select-none items-center justify-center rounded-md border border-zinc-800 bg-zinc-900 px-1 text-[10px] font-semibold text-zinc-400 shadow-sm hover:bg-zinc-800 hover:text-zinc-100 active:translate-y-px"
    >
      {children}
    </ActionKey>
  );
}

