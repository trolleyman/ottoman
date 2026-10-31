import { useEffect, useRef, useState } from "react";
import { haptic } from "./keyboardHaptics";
import { Modifier } from "./api";
import type { KeyboardSettings } from "./trackpadSettings";
import type { KeyDefinition } from "./keyboardData";

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
  children,
}: {
  modifier: Modifier;
  label: string;
  text: string;
  selected: Modifier[];
  onToggle: (modifier: Modifier) => void;
  children: React.ReactNode;
}) {
  const active = selected.includes(modifier);
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      aria-pressed={active}
      onPointerDown={(event) => event.preventDefault()}
      onClick={() => onToggle(modifier)}
      className={`flex h-9 min-w-10 flex-1 items-center justify-center gap-2 rounded-md border px-1 transition-colors ${
        active
          ? "border-blue-400/60 bg-blue-500 text-white shadow-[0_0_0_2px_rgba(59,130,246,.16)]"
          : "custom-keyboard-utility border-zinc-800 bg-zinc-900 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100"
      }`}
    >
      {children}
      <span className="text-[10px] font-semibold tracking-wide">{text}</span>
    </button>
  );
}

export function Key({
  definition,
  label,
  settings,
  onType,
}: {
  definition: KeyDefinition;
  label?: string;
  settings: KeyboardSettings;
  onType: (value: string, wasLongPress: boolean) => void;
}) {
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const held = useRef(false);

  useEffect(() => () => {
    if (timer.current) clearTimeout(timer.current);
  }, []);
  const [preview, setPreview] = useState(false);

  const clearTimer = () => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = null;
  };

  const start = (event: React.PointerEvent<HTMLButtonElement>) => {
    clearTimer();
    event.currentTarget.setPointerCapture(event.pointerId);
    held.current = false;
    if (!definition.alt) return;
    timer.current = setTimeout(() => {
      held.current = true;
      setPreview(true);
      haptic(settings.haptics, 16);
      onType(definition.alt!, true);
    }, settings.longPressDelayMs);
  };

  const finish = () => {
    clearTimer();
    setPreview(false);
    if (held.current) {
      held.current = false;
      return;
    }
    haptic(settings.haptics);
    onType(definition.key, false);
  };

  return (
    <button
      type="button"
      className="custom-keyboard-key relative flex min-w-0 flex-1 touch-none select-none items-center justify-center overflow-visible rounded-md border border-white/10 bg-zinc-600/95 text-[clamp(15px,4.5vw,20px)] font-medium text-white shadow-[0_1px_0_rgba(0,0,0,.7),0_2px_3px_rgba(0,0,0,.35)] active:translate-y-px active:bg-zinc-500"
      style={definition.grow ? { flexGrow: definition.grow } : undefined}
      aria-label={definition.alt ? `${definition.key}, hold for ${definition.alt}` : definition.key}
      onPointerDown={start}
      onPointerUp={finish}
      onPointerCancel={() => {
        clearTimer();
        setPreview(false);
        held.current = false;
      }}
      onContextMenu={(event) => event.preventDefault()}
    >
      <span>{label ?? definition.key}</span>
      {settings.symbolHints && definition.alt && (
        <span className="custom-keyboard-hint absolute right-1 top-0.5 text-[9px] font-semibold text-zinc-300/55">
          {definition.alt}
        </span>
      )}
      {preview && (
        <span className="absolute -top-12 left-1/2 z-10 flex h-12 w-10 -translate-x-1/2 items-center justify-center rounded-lg border border-white/20 bg-zinc-500 text-xl shadow-xl">
          {definition.alt}
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
    <button
      type="button"
      aria-label={label}
      onPointerDown={(event) => event.preventDefault()}
      onClick={onPress}
      className={`flex touch-none select-none items-center justify-center rounded-md border text-sm font-semibold shadow-[0_1px_0_rgba(0,0,0,.7),0_2px_3px_rgba(0,0,0,.3)] active:translate-y-px ${
        wide ? "min-w-[52px] flex-[1.4]" : "min-w-0 flex-1"
      } ${
        active
          ? "border-blue-400/50 bg-blue-500 text-white"
          : "custom-keyboard-utility border-white/5 bg-zinc-800 text-zinc-100 active:bg-zinc-700"
      }`}
    >
      {children}
    </button>
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
    <button
      type="button"
      aria-label={label}
      onPointerDown={(event) => event.preventDefault()}
      onClick={onPress}
      className="custom-keyboard-utility flex h-8 min-w-0 flex-1 touch-none select-none items-center justify-center rounded-md border border-zinc-800 bg-zinc-900 px-1 text-[10px] font-semibold text-zinc-400 shadow-sm hover:bg-zinc-800 hover:text-zinc-100 active:translate-y-px"
    >
      {children}
    </button>
  );
}

