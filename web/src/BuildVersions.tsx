import { useEffect, useState } from "react";

declare const __BUILD_REVISION__: string;

const webRevision = typeof __BUILD_REVISION__ === "string" ? __BUILD_REVISION__ : "unknown";
export function BuildVersions({ active, agent }: { active: boolean; agent?: string }) {
  const [controller, setController] = useState("unknown");
  useEffect(() => {
    if (!active) return;
    const operation = new AbortController();
    const timeout = setTimeout(() => operation.abort(), 5000);
    void fetch("/api/status", { cache: "no-store", signal: operation.signal }).then(async response => {
      if (!response.ok) throw Error();
      const state = await response.json() as { version?: unknown };
      if (!operation.signal.aborted) setController(typeof state.version === "string" ? state.version : "unknown");
    }).catch(() => { if (!operation.signal.aborted) setController("unavailable"); });
    return () => { clearTimeout(timeout); operation.abort(); };
  }, [active]);
  const same = controller === webRevision && agent === webRevision;
  return <footer className="remote-build-versions" aria-label="Build versions">{same
    ? <span>Build <code>{webRevision}</code></span>
    : <><span>Web <code>{webRevision}</code></span><span>Controller <code>{controller}</code></span><span>Agent <code>{agent || "offline"}</code></span></>}
  </footer>;
}
