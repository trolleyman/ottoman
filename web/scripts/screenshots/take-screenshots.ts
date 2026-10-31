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
// A failed interaction exits nonzero so screenshot checks cannot silently pass.
//
// Run by hand from web/:  bun scripts/screenshots/take-screenshots.ts

import { spawn, spawnSync, type ChildProcess } from 'node:child_process'
import { createServer } from 'node:net'
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { chromium, type Browser, type Page } from 'playwright'
import { checkKeyboardInteractions, type SentMessage } from './keyboard-checks'

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
  keyboard?: 'letters' | 'symbols' | 'symbol-search' | 'emoji' | 'emoji-search' | 'space-nav' | 'key-preview' | 'key-hold' | 'text-preview'
  remote?: boolean
  taller?: boolean
  settings?: boolean
  settingsTiming?: boolean
  scheme?: 'light' | 'dark'
  platform?: 'apple' | 'pc'
}

const SHOTS: Shot[] = [
  { name: 'pc-remote-taller-phone@2x', state: 'online', width: 402, height: 874, dpi: 2, tags: ['viewport::phone', 'feature::taller-keys'], remote: true, taller: true },
  { name: 'pc-remote-taller-short-phone@2x', state: 'online', width: 375, height: 667, dpi: 2, tags: ['viewport::short-phone', 'feature::taller-keys'], remote: true, taller: true },
  { name: 'pc-remote-phone@2x', state: 'online', width: 402, height: 874, dpi: 2, tags: ['viewport::phone', 'feature::remote'], remote: true },
  { name: 'pc-remote-desktop@2x', state: 'online', width: 1280, height: 900, dpi: 2, tags: ['viewport::desktop', 'feature::remote'], remote: true },
  { name: 'home-offline-phone', state: 'offline', width: 402, height: 874, dpi: 2, tags: ['viewport::phone', 'state::offline'] },
  { name: 'home-online-phone', state: 'online', width: 402, height: 874, dpi: 2, tags: ['viewport::phone', 'state::online'] },
  { name: 'trackpad-keyboard-dark-phone', state: 'online', width: 402, height: 874, dpi: 2, tags: ['viewport::phone', 'state::online', 'feature::keyboard', 'theme::dark', 'platform::pc'], keyboard: 'letters', scheme: 'dark', platform: 'pc' },
  { name: 'trackpad-keyboard-light-phone', state: 'online', width: 402, height: 874, dpi: 2, tags: ['viewport::phone', 'state::online', 'feature::keyboard', 'theme::light', 'platform::apple'], keyboard: 'letters', scheme: 'light', platform: 'apple' },
  { name: 'trackpad-key-preview-phone', state: 'online', width: 402, height: 874, dpi: 2, tags: ['viewport::phone', 'feature::key-preview'], keyboard: 'key-preview', platform: 'pc' },
  { name: 'trackpad-key-hold-phone', state: 'online', width: 402, height: 874, dpi: 2, tags: ['viewport::phone', 'feature::key-hold'], keyboard: 'key-hold', platform: 'pc' },
  { name: 'trackpad-text-preview-phone', state: 'online', width: 402, height: 874, dpi: 2, tags: ['viewport::phone', 'feature::text-preview'], keyboard: 'text-preview', platform: 'pc' },
  { name: 'trackpad-space-navigation-phone', state: 'online', width: 402, height: 874, dpi: 2, tags: ['viewport::phone', 'state::online', 'feature::space-navigation'], keyboard: 'space-nav', platform: 'pc' },
  { name: 'trackpad-symbols-phone', state: 'online', width: 402, height: 874, dpi: 2, tags: ['viewport::phone', 'state::online', 'feature::symbols', 'platform::pc'], keyboard: 'symbols', platform: 'pc' },
  { name: 'trackpad-symbol-search-phone', state: 'online', width: 402, height: 874, dpi: 2, tags: ['viewport::phone', 'state::online', 'feature::symbol-search', 'platform::pc'], keyboard: 'symbol-search', platform: 'pc' },
  { name: 'trackpad-emoji-phone', state: 'online', width: 402, height: 874, dpi: 2, tags: ['viewport::phone', 'state::online', 'feature::emoji'], keyboard: 'emoji' },
  { name: 'trackpad-emoji-search-phone', state: 'online', width: 402, height: 874, dpi: 2, tags: ['viewport::phone', 'state::online', 'feature::emoji-search'], keyboard: 'emoji-search' },
  { name: 'trackpad-settings-phone', state: 'online', width: 402, height: 874, dpi: 2, tags: ['viewport::phone', 'state::online', 'feature::settings'], settings: true },
  { name: 'trackpad-settings-timing-phone', state: 'online', width: 402, height: 874, dpi: 2, tags: ['viewport::phone', 'state::online', 'feature::settings-timing'], settingsTiming: true },
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
        colorScheme: shot.scheme || 'dark',
        locale: 'en-GB',
        hasTouch: shot.width < 768,
        ...(shot.platform === 'pc' ? {
          // Deliberately use an Apple browser for a Windows remote; modifiers
          // must follow the agent status, not this user agent.
          userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 15_0) AppleWebKit/605.1.15 Version/18.0 Safari/605.1.15',
        } : {}),
      })
      await ctx.addInitScript((preferences) => {
        localStorage.setItem('ottoman-trackpad-settings', JSON.stringify({ ...preferences, keyboardLayout: 'en-GB' }))
      }, { keyboardTheme: shot.scheme || 'dark', tallerKeys: shot.taller || false })
      const page = await ctx.newPage()
      const sent: SentMessage[] = []
      page.on('websocket', (socket) => socket.on('framesent', (frame) => {
        try { sent.push({ message: JSON.parse(frame.payload.toString()) as SentMessage['message'], time: Date.now() }) } catch { /* ignore non-JSON frames */ }
      }))
      if (shot.platform) await page.route('**/api/status/agent', async (route) => {
        const response = await route.fetch()
        if (!response.ok()) { await route.fulfill({ response }); return }
        const data = await response.json() as Record<string, unknown>
        await route.fulfill({ response, json: { ...data, os: shot.platform === 'apple' ? 'darwin' : 'windows' } })
      })
      await page.goto(base + (shot.remote ? "/remote" : ""), { waitUntil: 'networkidle' })
      await settle(page)
      let heldSpace = false
      let heldKey = false
      if (shot.remote) {
        await page.getByLabel(/Trackpad\. Tap to click/).focus()
        for (const letter of ["h", "e"]) await page.locator(`[data-letter="${letter}"]`).click()
        await page.getByLabel("Word suggestions").getByRole("button").first().waitFor()
        if (await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)) throw new Error("Remote page overflows horizontally")
        if (shot.taller) {
          const enter = page.getByRole('button', { name: 'Enter', exact: true })
          await enter.scrollIntoViewIfNeeded()
          const enterBox = await enter.boundingBox()
          if (!enterBox || enterBox.y < 0 || enterBox.y + enterBox.height > shot.height) throw new Error('Tall keyboard Enter key is not reachable')
        }
      } else if (shot.keyboard) {
        const openKeyboard = page.getByLabel(/Trackpad\. Tap to click/)
        await openKeyboard.scrollIntoViewIfNeeded()
        await openKeyboard.focus()
        if (shot.keyboard === 'key-preview' || shot.keyboard === 'key-hold') {
          const key = page.locator('[data-letter="w"]')
          const box = await key.boundingBox()
          if (!box) throw new Error('Missing E key')
          await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
          await page.mouse.down()
          await page.waitForTimeout(shot.keyboard === 'key-hold' ? 480 : 80)
          heldKey = true
        } else if (shot.keyboard === 'text-preview') {
          await page.getByRole('button', { name: 'Keyboard settings', exact: true }).click()
          await page.getByRole('button', { name: 'Device', exact: true }).click()
          await page.getByLabel(/Trackpad\. Tap to click/).focus()
          await page.getByLabel('Type on the remote computer with the device keyboard').fill('This preview stays while I type. It wraps whole words and grows upwards from the bottom of the trackpad, without trimming letters or fading away on a timer.\n\nBritish symbols: £, #, backslash, ?, and apostrophe. More typing keeps pushing the earlier lines upwards. '.repeat(3))
          await page.getByRole('button', { name: 'Open keyboard settings' }).click()
          await page.getByRole('button', { name: 'Ottoman', exact: true }).click()
          await page.getByRole('button', { name: 'Back to keyboard', exact: true }).click()
        } else if (shot.keyboard === 'letters' || shot.keyboard === 'space-nav') {
          await page.getByRole('button', { name: 'Control' }).click()
          await page.getByRole('button', { name: 'Shift' }).click()
          await page.getByRole('button', { name: 'c, hold for ,' }).click()
          await page.getByRole('button', { name: 'Control' }).click()
          for (const key of ['h', 'e', 'l', 'l', 'o'])
            await page.locator(`[data-letter="${key}"]`).click()
          await page.getByRole('button', { name: 'Tab' }).click()
          for (const key of ['Escape', 'Home', 'Arrow left', 'Arrow up', 'Arrow down', 'Arrow right', 'End', 'Delete'])
            await page.getByRole('button', { name: key, exact: true }).click()
          if (shot.platform === 'pc') {
            if (shot.keyboard === 'space-nav') {
              await page.getByLabel(/Trackpad\. Tap to click/).evaluate((element) =>
                element.scrollIntoView({ block: 'start' }),
              )
              await page.waitForTimeout(150)
            }
            const space = page.getByRole('button', { name: 'Space. Hold and drag to move the cursor' })
            const box = await space.boundingBox()
            if (!box) throw new Error('Space bar has no bounding box')
            await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
            await page.mouse.down()
            await page.waitForTimeout(360)
            if (shot.keyboard === 'space-nav') heldSpace = true
            else {
              await page.mouse.move(box.x + box.width / 2 + 40, box.y + box.height / 2)
              await page.mouse.up()
            }
          }
        } else if (shot.keyboard === 'emoji' || shot.keyboard === 'emoji-search') {
          await page.getByRole('button', { name: 'Emoji' }).click()
          await page.getByRole('region', { name: 'All emoji' }).waitFor()
          if (shot.keyboard === 'emoji-search') {
            await page.getByRole('button', { name: 'Search Emoji' }).click()
            for (const key of ['l', 'o', 'v', 'e'])
              await page.locator(`[data-letter="${key}"]`).click()
          }
        } else {
          await page.getByRole('button', { name: 'Symbols' }).click()
          if (shot.keyboard === 'symbol-search') {
            await page.getByRole('button', { name: 'Search Symbols' }).click()
            for (const key of ['p', 'o', 'u', 'n', 'd'])
              await page.locator(`[data-letter="${key}"]`).click()
            await page.getByRole('button', { name: 'Type £' }).waitFor()
          } else {
            await page.getByRole('button', { name: '£', exact: true }).waitFor()
          }
        }
        if (!heldSpace && !heldKey) {
          await page.getByLabel(/Trackpad\. Tap to click/).evaluate((element) =>
            element.scrollIntoView({ block: 'start' }),
          )
          await page.waitForTimeout(150)
        }
      } else if (shot.settings || shot.settingsTiming) {
        const openKeyboard = page.getByLabel(/Trackpad\. Tap to click/)
        await openKeyboard.scrollIntoViewIfNeeded()
        await openKeyboard.focus()
        await page.getByRole('button', { name: 'Keyboard settings' }).click()
        if (shot.settingsTiming) {
          await page.getByText('Timing', { exact: true }).scrollIntoViewIfNeeded()
          await page.getByLabel('Space cursor hold milliseconds').fill('500')
          await page.getByLabel(/Trackpad\. Tap to click/).evaluate((element) =>
            element.scrollIntoView({ block: 'start' }),
          )
        }
        await page.waitForTimeout(150)
      }
      const file = join(OUT, `${shot.name}.png`)
      await page.screenshot({ path: file, fullPage: !shot.remote && !shot.keyboard && !shot.settings && !shot.settingsTiming })
      if (shot.keyboard === 'key-hold') await page.locator('.custom-keyboard-shell').screenshot({ path: join(OUT, 'keyboard-press@2x.png') })
      if (shot.name === 'trackpad-keyboard-dark-phone') await checkKeyboardInteractions(page, sent)
      if (shot.keyboard) {
        if (heldSpace || heldKey) await page.mouse.up()
        await page.getByRole('button', { name: 'Keyboard settings' }).click()
        await page.getByRole('button', { name: 'Device', exact: true }).click()
        await page.getByLabel(/Trackpad\. Tap to click/).focus()
        await page.waitForFunction(() => document.activeElement?.getAttribute("aria-label") === "Type on the remote computer with the device keyboard")
        const nativeInput = page.getByLabel('Type on the remote computer with the device keyboard')
        if (!await nativeInput.evaluate((element) => document.activeElement === element))
          throw new Error('Device keyboard input did not receive focus')
        await page.getByRole('button', { name: 'Open keyboard settings' }).click()
        await page.getByRole('button', { name: 'Ottoman', exact: true }).click()
        await page.getByLabel(/Trackpad\. Tap to click/).waitFor()
      }
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
name = "Samsung Odyssey G9"
edid = "SAM:0F9C"
width = 5120
height = 1440
refresh_rate = 240.0
position_x = 0
position_y = 0
primary = true
`

main().catch((err) => {
  console.error(err)
  process.exitCode = 1
})
