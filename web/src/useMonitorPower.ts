import { useEffect, useRef, useState } from "react";
import { useStore } from "./store";

// Cadence + safety caps for confirming a power toggle. There's no direct
// power-state read, so after sending the command we poll probeMonitorPower
// until the monitor reports the target state:
//   • turning ON  → confirmed once it answers again (responding === true)
//   • turning OFF → confirmed once it STOPS answering (responding === false);
//     while it keeps answering we stay in the loading state, since a monitor
//     that's still on hasn't gone to standby yet.
// (A DDC probe against an off monitor blocks ~2s on ddcutil's own timeout, so
// the loop paces itself.) If the cap passes without confirmation the monitor
// never reached the target, so we revert the optimistic switch.
const POLL_MS = 700;
const CONFIRM_ON_MAX_MS = 20_000; // waking a display (or booting a TV) can be slow
const CONFIRM_OFF_MAX_MS = 15_000; // give up if it keeps answering this long

export function useMonitorPower(edid: string, initialOn: boolean) {
  const setMonitorPower = useStore((s) => s.setMonitorPower);
  const probeMonitorPower = useStore((s) => s.probeMonitorPower);

  const [on, setOn] = useState(initialOn);
  const [loading, setLoading] = useState(false);
  const runId = useRef(0);
  const mounted = useRef(true);
  useEffect(() => () => { mounted.current = false; }, []);

  const toggle = async (target: boolean) => {
    const id = ++runId.current; // supersedes any in-flight poll
    setOn(target);
    setLoading(true);
    // setMonitorPower surfaces its own errors; poll regardless of the result.
    await setMonitorPower(edid, target);

    const deadline = Date.now() + (target ? CONFIRM_ON_MAX_MS : CONFIRM_OFF_MAX_MS);
    while (Date.now() < deadline) {
      const responding = await probeMonitorPower(edid);
      if (runId.current !== id || !mounted.current) return; // superseded / unmounted
      if (responding === target) {
        setLoading(false);
        return;
      }
      await sleep(POLL_MS);
      if (runId.current !== id || !mounted.current) return;
    }
    // Never confirmed within the cap — reflect that it didn't reach the target.
    setOn(!target);
    setLoading(false);
  };

  return { on, loading, toggle };
}

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}
