/// <reference types="node" />
//
// Runs ESLint and converts every finding into a Hydra WARNING marker (the
// .hydra/config.toml "web" runner, type = "stdout"). Both error- and
// warning-severity messages map to ::hydra:test:warn::, so lint problems show as
// amber ⚠ diagnostics in the tests panel but never fail the run or gate the
// merge. The tree is lint-clean today; this keeps it visible (and honest) if it
// regresses, without turning every merge red the way `eslint`'s own error exit
// would. Type-checking (tsc-markers) remains the gating web check.
//
// Run by hand from web/:  bun scripts/hydra/eslint-markers.ts
import { spawnSync } from 'node:child_process'

// -f json gives one entry per linted file with a messages[] array; we always
// exit 0 here so lint stays advisory regardless of eslint's own exit code.
const res = spawnSync('node_modules/.bin/eslint', ['.', '-f', 'json'], {
  encoding: 'utf8',
  maxBuffer: 64 * 1024 * 1024,
})

const esc = (s: string) =>
  s.replace(/\\/g, '\\\\').replace(/\n/g, '\\n').replace(/\t/g, '\\t').replace(/\r/g, '\\r')

interface LintMessage {
  line?: number
  column?: number
  ruleId?: string | null
  message: string
  severity: number
}
interface LintResult {
  filePath: string
  messages: LintMessage[]
}

let results: LintResult[] = []
try {
  results = JSON.parse(res.stdout || '[]') as LintResult[]
} catch {
  // eslint crashed before emitting JSON (bad config, parser blow-up). Surface its
  // output in the build log but don't gate — lint is advisory here.
  if (res.stderr) console.log(res.stderr)
  process.exit(0)
}

let count = 0
for (const file of results) {
  // filePaths are absolute; make them repo-relative (…/web/<rel>) so Hydra links them.
  const rel = file.filePath.replace(/^.*\/web\//, 'web/')
  for (const m of file.messages) {
    count++
    const loc = `${rel}:${m.line ?? 0}:${m.column ?? 0}`
    const rule = m.ruleId ?? (m.severity === 2 ? 'error' : 'warning')
    console.log(`::hydra:test:warn:: ${loc} › ${rule} | ${esc(m.message)}`)
  }
}

if (count === 0) console.log('::hydra:test:pass:: web › lint')

// Lint is advisory: warnings never gate the merge.
process.exit(0)
