import { execFileSync } from 'node:child_process'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react-swc'
import tailwindcss from '@tailwindcss/vite'

function buildRevision() {
  try {
    const revision = execFileSync('git', ['rev-parse', '--short=12', 'HEAD'], { encoding: 'utf8' }).trim()
    const dirty = execFileSync('git', ['status', '--porcelain'], { encoding: 'utf8' }).trim()
    return revision + (dirty ? '-dirty' : '')
  } catch { return 'unknown' }
}

export default defineConfig({
  define: { __BUILD_REVISION__: JSON.stringify(buildRevision()) },
  plugins: [react(), tailwindcss()],
})
