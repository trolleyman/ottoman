/// <reference types="node" />
//
// Runs the frontend type-check and converts tsc's diagnostics into Hydra
// streaming test markers (the .hydra/config.toml "web" runner, type = "stdout").
// Each error becomes a fail marker located at file:line:col; a clean run emits a
// single passing "typecheck" case so the tests panel shows a real green check
// instead of "(command exited 0)".
//
// Run by hand from web/:  bun scripts/hydra/tsc-markers.ts
import { spawnSync } from 'node:child_process'

// --force so the check runs fresh every time (tsc -b otherwise skips unchanged
// project references via .tsbuildinfo and prints nothing); --pretty false keeps
// each diagnostic on one line so it maps cleanly to a single marker.
const res = spawnSync('node_modules/.bin/tsc', ['-b', '--force', '--pretty', 'false'], {
  encoding: 'utf8',
})
const out = `${res.stdout ?? ''}${res.stderr ?? ''}`

// tsc diagnostic line, e.g. "src/App.tsx(12,5): error TS2322: Type '...'."
const diag = /^(.+?)\((\d+),(\d+)\):\s+error\s+(TS\d+):\s+(.*)$/
const esc = (s: string) =>
  s.replace(/\\/g, '\\\\').replace(/\n/g, '\\n').replace(/\t/g, '\\t').replace(/\r/g, '\\r')

let errors = 0
for (const line of out.split('\n')) {
  const m = diag.exec(line)
  if (!m) {
    if (line.trim()) console.log(line) // keep other tsc chatter in the build log
    continue
  }
  errors++
  const [, file, ln, col, code, msg] = m
  // Paths are relative to web/ (cwd); prefix so Hydra's location is repo-relative.
  console.log(`::hydra:test:fail:: web/${file}:${ln}:${col} › ${code} | ${esc(msg)}`)
}

console.log('::hydra:test:total:: 1')
if (errors === 0) console.log('::hydra:test:pass:: web › typecheck')

process.exit(res.status ?? (errors > 0 ? 1 : 0))
