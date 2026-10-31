export function supportsHaptics(): boolean {
  return typeof navigator !== "undefined" && typeof navigator.vibrate === "function";
}

export function haptic(enabled: boolean, pattern: number | number[] = 8) {
  if (!enabled || !supportsHaptics()) return;
  navigator.vibrate(pattern);
}
