/// <reference types="node" />
//
// Diff-viewer artifact generator for Ottoman's web UI (.hydra/config.toml
// [[artifacts]] "screenshots"). Hydra runs this against both sides of a diff
// and surfaces the rendered PNGs that changed, so visual tweaks to the UI show
// up side-by-side in the head's diff viewer.
//
// What it does: build the frontend, boot `ottoman controller simulate` (which
// serves the REAL frontend against mocked API data and needs no browser auth —
// its CheckAuth always returns authenticated), and screenshot the app with a
// headless Chromium in a few states/viewports.
//
// Contract (set by Hydra's artifact runner; all optional when run by hand):
//   HYDRA_ARTIFACT_OUTPUT  dir to write <name>.png (+ <name>.png.meta) into
//   HYDRA_ARTIFACT_SOURCE  the checkout dir (repo root); the cwd is web/
//   HYDRA_ARTIFACT_REF     the resolved ref being rendered (informational)
//
// Alongside each <name>.png we write a <name>.png.meta JSON sidecar
// ({"tags":[...],"dpi":N}) that the diff viewer shows as labels/filters.
//
// Progress: each step prints a one-line "::hydra:progress::" marker that Hydra
// surfaces as the live loading text (and, once seen, stops treating ordinary
// stdout as progress, so bun/vite/go chatter can't hijack the header).
//
// The artifact is deliberately best-effort: a build break or a boot failure
// logs and exits 0 with no images rather than failing the head.
//
// Run by hand from web/:  bun scripts/screenshots/take-screenshots.ts

import { spawn, spawnSync, type ChildProcess } from 'node:child_process'
import { createServer } from 'node:net'
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { chromium, type Browser, type Page } from 'playwright'

const progress = '::hydra:progress::'
const p = (msg: string) => console.log(progress + msg)

// web/ is the cwd; the repo root is one level up (or HYDRA_ARTIFACT_SOURCE).
const WEB = process.cwd()
const ROOT = process.env.HYDRA_ARTIFACT_SOURCE || join(WEB, '..')
const OUT = process.env.HYDRA_ARTIFACT_OUTPUT || join(WEB, 'scripts', 'screenshots', 'out')
mkdirSync(OUT, { recursive: true })

// A screenshot to capture: which app state to put the sim in, the viewport, and
// the device-scale factor (phone shots render at dpi 2 for crispness; the diff
// grid lays a tile out by its logical width, so dpi 2 looks the same size, only
// sharper).
type Shot = {
  name: string
  state: 'offline' | 'online'
  width: number
  height: number
  dpi: number
  tags: string[]
}

const SHOTS: Shot[] = [
  { name: 'home-offline-phone', state: 'offline', width: 402, height: 874, dpi: 2, tags: ['viewport::phone', 'state::offline'] },
  { name: 'home-online-phone', state: 'online', width: 402, height: 874, dpi: 2, tags: ['viewport::phone', 'state::online'] },
  { name: 'home-online-desktop', state: 'online', width: 1280, height: 900, dpi: 1, tags: ['viewport::desktop', 'state::online'] },
]

async function main() {
  // 1. Build the frontend so the simulated server embeds the current UI.
  p('Building the frontend...')
  if (!run('bun', ['run', 'build'], WEB)) {
    p('Frontend build failed — no screenshots')
    return
  }

  // 2. Build the ottoman binary (embeds the dist we just built).
  const work = mkdtempSync(join(tmpdir(), 'ottoman-shots-'))
  const bin = join(work, 'ottoman')
  p('Building the ottoman binary...')
  if (!run('go', ['build', '-o', bin, './cmd/ottoman'], ROOT)) {
    p('Binary build failed — no screenshots')
    rmSync(work, { recursive: true, force: true })
    return
  }

  // 3. Write throwaway sim configs (a free port, sample layouts for a populated
  //    UI). The keys are the config structs' json tags (viper unmarshals with
  //    TagName "json").
  const port = await freePort()
  const ctrlCfg = join(work, 'controller.toml')
  const agentCfg = join(work, 'agent.toml')
  writeFileSync(ctrlCfg, controllerToml(port))
  writeFileSync(agentCfg, AGENT_TOML)

  // 4. Boot the simulated controller.
  p('Starting the simulated controller...')
  const server: ChildProcess = spawn(
    bin,
    ['--config', ctrlCfg, 'controller', 'simulate', '--agent-config', agentCfg],
    { cwd: ROOT, stdio: ['ignore', 'inherit', 'inherit'] },
  )
  const base = `http://127.0.0.1:${port}`
  let browser: Browser | undefined
  try {
    await waitForServer(`${base}/health`)

    // 5. Capture each shot.
    browser = await chromium.launch()
    let n = 0
    for (const shot of SHOTS) {
      n++
      p(`${shot.name}.png ${n}/${SHOTS.length}`)
      await setState(base, shot.state)
      const ctx = await browser.newContext({
        viewport: { width: shot.width, height: shot.height },
        deviceScaleFactor: shot.dpi,
        colorScheme: 'dark',
      })
      const page = await ctx.newPage()
      await page.goto(base, { waitUntil: 'networkidle' })
      await settle(page)
      const file = join(OUT, `${shot.name}.png`)
      await page.screenshot({ path: file, fullPage: true })
      writeFileSync(`${file}.meta`, JSON.stringify({ tags: shot.tags, dpi: shot.dpi }))
      await ctx.close()
    }
    p(`Captured ${SHOTS.length} screenshots`)
  } finally {
    await browser?.close().catch(() => {})
    server.kill('SIGKILL')
    rmSync(work, { recursive: true, force: true })
  }
}

// --- helpers ---------------------------------------------------------------

// run executes a command synchronously, streaming its output; returns success.
function run(cmd: string, args: string[], cwd: string): boolean {
  const r = spawnSync(cmd, args, { cwd, stdio: 'inherit' })
  return r.status === 0
}

// freePort asks the OS for an unused TCP port (bind :0, read it back, release).
function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const srv = createServer()
    srv.once('error', reject)
    srv.listen(0, '127.0.0.1', () => {
      const addr = srv.address()
      const port = typeof addr === 'object' && addr ? addr.port : 0
      srv.close(() => resolve(port))
    })
  })
}

// waitForServer polls an URL until it answers 2xx or the deadline passes.
async function waitForServer(url: string, timeoutMs = 30_000): Promise<void> {
  const deadline = Date.now() + timeoutMs
  for (;;) {
    try {
      const res = await fetch(url)
      if (res.ok) return
    } catch {
      /* not up yet */
    }
    if (Date.now() > deadline) throw new Error(`server never came up at ${url}`)
    await sleep(200)
  }
}

// setState drives the sim's admin endpoint to offline/online so we can capture
// both the Wake-on-LAN prompt and the populated dashboard.
async function setState(base: string, state: 'offline' | 'online'): Promise<void> {
  const res = await fetch(`${base}/api/sim/set-state`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ state }),
  })
  if (!res.ok) throw new Error(`set-state ${state} failed: ${res.status}`)
}

// settle waits for the app to finish its initial data fetches and any fonts.
async function settle(page: Page): Promise<void> {
  await page.evaluate(() => document.fonts?.ready).catch(() => {})
  await page.waitForTimeout(500)
}

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms))
}

function controllerToml(port: number): string {
  return [
    '[controller]',
    `listen_address = ":${port}"`,
    'auth_token = "simulated"',
    '',
    '[controller.agent]',
    'mac_address = "AA:BB:CC:DD:EE:FF"',
    'ip_address = "192.168.1.50"',
    'port = 17294',
    '',
  ].join('\n')
}

// A sample agent config so the simulated UI shows real layouts and monitors
// (the sim derives its monitor list from these layouts).
const AGENT_TOML = `[agent]
listen_address = ":17294"
auth_token = "simulated"

[[agent.layouts]]
id = "desktop"
name = "Desktop"
emoji = "🖥️"
aliases = ["d", "main"]
[[agent.layouts.monitors]]
name = "Dell U2720Q"
edid = "DEL:D0A2"
width = 3840
height = 2160
refresh_rate = 60.0
position_x = 0
position_y = 0
primary = true
[[agent.layouts.monitors]]
name = "LG UltraGear"
edid = "GSM:5B9E"
width = 2560
height = 1440
refresh_rate = 144.0
position_x = 3840
position_y = 0

[[agent.layouts]]
id = "movie"
name = "Movie"
emoji = "🎬"
aliases = ["m", "tv"]
[[agent.layouts.monitors]]
name = "LG OLED TV"
edid = "GSM:C001"
width = 3840
height = 2160
refresh_rate = 120.0
position_x = 0
position_y = 0
primary = true
`

main().catch((err) => {
  console.error(err)
  // Best-effort: never fail the head over a screenshot.
  process.exit(0)
})
