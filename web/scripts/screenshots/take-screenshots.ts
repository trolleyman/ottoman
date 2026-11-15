import { openSettings } from "../tests/settings-helpers";
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
type Shot = { name: string; state: 'online' | 'offline'; width: number; height: number; view?: string; light?: boolean }
const SHOTS: Shot[] = [
  {name:'pc-remote-phone@2x',state:'online',width:402,height:874},
  {name:'pc-remote-short-phone@2x',state:'online',width:375,height:667},
  {name:'pc-remote-desktop@2x',state:'online',width:1280,height:900},
  {name:'pc-remote-offline@2x',state:'offline',width:402,height:874},
  {name:'pc-remote-menu@2x',state:'online',width:402,height:874,view:'menu'},
  {name:'pc-remote-settings@2x',state:'online',width:402,height:874,view:'settings'},
  {name:'pc-remote-gestures@2x',state:'online',width:402,height:874,view:'gestures'},
  {name:'pc-remote-emoji@2x',state:'online',width:402,height:874,view:'emoji'},
  {name:'pc-remote-symbols@2x',state:'online',width:402,height:874,view:'symbols'},
  {name:'pc-remote-clipboard@2x',state:'online',width:402,height:874,view:'clipboard'},
  {name:'pc-remote-displays@2x',state:'online',width:402,height:874,view:'Displays'},
  {name:'pc-remote-connection@2x',state:'online',width:402,height:874,view:'Connection & power'},
  {name:'pc-remote-light@2x',state:'online',width:402,height:874,light:true},
]

async function main() {
  p('Building the remote-first UI...')
  if (!run('bun',['run','build'],WEB)) throw Error('Frontend build failed')
  const work=mkdtempSync(join(tmpdir(),'ottoman-shots-'))
  const bin=join(work,'ottoman')
  if (!run('go',['build','-o',bin,'./cmd/ottoman'],ROOT)) throw Error('Simulator build failed')
  const port=await freePort()
  const ctrlCfg=join(work,'controller.toml'), agentCfg=join(work,'agent.toml')
  writeFileSync(ctrlCfg,controllerToml(port));writeFileSync(agentCfg,AGENT_TOML)
  const server:ChildProcess=spawn(bin,['--config',ctrlCfg,'controller','simulate','--agent-config',agentCfg],{cwd:ROOT,stdio:['ignore','inherit','inherit']})
  const base=`http://127.0.0.1:${port}`
  let browser:Browser|undefined
  try {
    await waitForServer(`${base}/health`)
    browser=await chromium.launch({executablePath:process.env.PLAYWRIGHT_EXECUTABLE_PATH})
    for(const shot of SHOTS){
      p(`${shot.name}.png`)
      await setState(base,shot.state)
      const ctx=await browser.newContext({viewport:{width:shot.width,height:shot.height},deviceScaleFactor:2,colorScheme:shot.light?'light':'dark',locale:'en-GB',hasTouch:shot.width<768})
      const page=await ctx.newPage()
      try {
        await page.addInitScript(light=>localStorage.setItem('ottoman-trackpad-settings',JSON.stringify({mirrorFocusedText:true,keyboardLayout:'en-GB',keyboardTheme:light?'light':'dark'})),!!shot.light)
        await page.goto(base,{waitUntil:'networkidle'})
        if(shot.state==='online')await page.locator('.remote-connection i[data-connected="true"]').waitFor()
        else await page.locator('.custom-keyboard-shell[data-disconnected]').waitFor()
        await settle(page)
        if(['menu','settings','gestures','Displays','Connection & power'].includes(shot.view||'')){
          if(shot.view==='settings'||shot.view==='gestures'){
            await openSettings(page)
            if(shot.view==='gestures')await page.getByRole('tab',{name:'Trackpad',exact:true}).click()
          }else{
            await page.getByRole('button',{name:'Open menu',exact:true}).click()
            if(shot.view!=='menu')await page.getByRole('button',{name:shot.view,exact:true}).click()
          }
        }
        if(shot.view==='symbols'||shot.view==='emoji'){
          await page.getByRole('button',{name:'Symbols',exact:true}).click()
          if(shot.view==='emoji'){await page.getByRole('button',{name:'Emoji',exact:true}).click();await page.getByRole('region',{name:'All emoji',exact:true}).waitFor()}
        }
        if(shot.view==='clipboard')await page.getByRole('button',{name:'Clipboard',exact:true}).click()
        if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error(`${shot.name} overflows horizontally`)
        const file=join(OUT,`${shot.name}.png`)
        await page.screenshot({path:file})
        writeFileSync(`${file}.meta`,JSON.stringify({tags:[`viewport::${shot.width<768?'phone':'desktop'}`,`state::${shot.state}`,`feature::${shot.view||'remote'}`],dpi:2}))
      }finally{await ctx.close()}
    }
  }finally{await browser?.close();server.kill('SIGKILL');rmSync(work,{recursive:true,force:true})}
}

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
    `listen_address = "127.0.0.1:${port}"`,
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
