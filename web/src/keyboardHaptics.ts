export function haptic(enabled: boolean, pattern: number | number[] = 8) {
  if (!enabled) return;
  if ("vibrate" in navigator && navigator.vibrate(pattern)) return;

  // Safari 18+ gives `<input type="checkbox" switch>` a native Taptic tap.
  // Trigger one from the same user gesture as the key press. Older iOS builds
  // ignore the non-standard switch attribute, which is a safe silent fallback.
  if (/iPhone|iPad|iPod/.test(navigator.userAgent)) {
    const id = "ottoman-haptic-switch";
    let pulse = document.getElementById(id) as HTMLInputElement | null;
    if (!pulse) {
      pulse = document.createElement("input");
      pulse.id = id;
      pulse.type = "checkbox";
      pulse.setAttribute("switch", "");
      pulse.setAttribute("aria-hidden", "true");
      pulse.tabIndex = -1;
      Object.assign(pulse.style, {
        position: "fixed",
        width: "1px",
        height: "1px",
        opacity: "0",
        pointerEvents: "none",
      });
      document.body.appendChild(pulse);
    }
    pulse.click();
  }
}

