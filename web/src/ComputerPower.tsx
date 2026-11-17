import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Power, RotateCcw, X } from "lucide-react";
import { useStore } from "./store";

export function ComputerPower() {
  const { agentStatus: status, agentInfo, wake, shutdown, reboot } = useStore();
  const [open, setOpen] = useState(false);
  const bootTarget = agentInfo?.os === "windows" ? "linux" : "windows";
  const bootLabel = bootTarget === "linux" ? "Linux" : "Windows";
  const [busy, setBusy] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const popup = useRef<HTMLDivElement>(null);
  const online = status === "online";
  const offline = status === "offline";
  const pending = busy || (!online && !offline);
  useEffect(() => {
    if (!open) return;
    popup.current?.querySelector<HTMLButtonElement>("button")?.focus();
    const button = trigger.current;
    return () => button?.focus();
  }, [open]);
  const run = async (action: "shutdown" | "restart" | "wake") => {
    if (pending || (action === "wake" ? !offline : !online)) return;
    setBusy(true);
    try {
      if (action === "wake") await wake("linux");
      else if (action === "shutdown") await shutdown();
      else await reboot(bootTarget);
      setOpen(false);
    } finally { setBusy(false); }
  };
  return <>
    <button ref={trigger} className="remote-pc-power" aria-label="Computer power" aria-haspopup="dialog" aria-expanded={open} onClick={() => setOpen(true)}><Power /></button>
    {open && createPortal(<div className="computer-power-overlay" onClick={event => { if (event.target === event.currentTarget && !busy) setOpen(false); }}>
      <div ref={popup} className="computer-power-dialog" role="dialog" aria-modal="true" aria-labelledby="computer-power-title" onKeyDown={event => {
        event.stopPropagation();
        if (event.key === "Escape" && !busy) { event.preventDefault(); setOpen(false); }
        if (event.key === "Tab") {
          const buttons = Array.from(popup.current!.querySelectorAll<HTMLButtonElement>("button:not(:disabled)"));
          if (event.shiftKey && document.activeElement === buttons[0]) { event.preventDefault(); buttons.at(-1)?.focus(); }
          else if (!event.shiftKey && document.activeElement === buttons.at(-1)) { event.preventDefault(); buttons[0]?.focus(); }
        }
      }}>
        <header><h2 id="computer-power-title">{agentInfo?.hostname || "Computer"}</h2><button aria-label="Close computer power" disabled={busy} onClick={() => setOpen(false)}><X /></button></header>
        {online ? <><button className="computer-power-action computer-power-shutdown" disabled={pending} onClick={() => void run("shutdown")}><Power />Shut down</button><button className="computer-power-action" disabled={pending} onClick={() => void run("restart")}><RotateCcw />Restart to {bootLabel}</button></> : offline ? <button className="computer-power-action" disabled={pending} onClick={() => void run("wake")}><Power />Turn on</button> : <p role="status">{status === "waking" ? "Turning on…" : status === "shutting_down" ? "Shutting down…" : "Checking connection…"}</p>}
        <button className="computer-power-cancel" disabled={busy} onClick={() => setOpen(false)}>Cancel</button>
      </div>
    </div>, document.body)}
  </>;
}
