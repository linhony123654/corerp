import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdtemp, mkdir, readFile, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'

const projectRoot = resolve(import.meta.dirname, '..')
const pinned = JSON.parse(await readFile(join(projectRoot, 'clients/sillytavern/host-fixture.json'), 'utf8'))

function option(name, fallback = '') {
  const i = process.argv.indexOf(name)
  return i >= 0 ? process.argv[i + 1] : fallback
}

const requestedRoot = option('--root')
const hostRoot = requestedRoot ? resolve(requestedRoot) : await mkdtemp(join(tmpdir(), 'corerp-rp7-host-'))
const port = Number(option('--port', '4187'))
assert.ok(Number.isInteger(port) && port >= 1024 && port <= 65535, '--port must be an unprivileged TCP port')
await mkdir(hostRoot, { recursive: true })

const packed = JSON.parse(execFileSync('npm', ['pack', `${pinned.package}@${pinned.version}`, '--ignore-scripts', '--json'], {
  cwd: hostRoot,
  encoding: 'utf8',
  maxBuffer: 32 * 1024 * 1024,
}))
assert.equal(packed.length, 1, 'npm pack returned an unexpected artifact count')
assert.equal(packed[0].version, pinned.version, 'npm artifact version differs from the pinned host')
assert.equal(packed[0].integrity, pinned.integrity, 'npm artifact integrity differs from the pinned host')

execFileSync('tar', ['-xzf', join(hostRoot, packed[0].filename), '-C', hostRoot], { stdio: 'pipe' })
const packageRoot = join(hostRoot, 'package')
const packageJSON = JSON.parse(await readFile(join(packageRoot, 'package.json'), 'utf8'))
assert.equal(packageJSON.version, pinned.version)
execFileSync('npm', ['install', '--omit=dev', '--ignore-scripts', '--no-audit', '--no-fund'], {
  cwd: packageRoot,
  stdio: 'inherit',
})

let config = await readFile(join(packageRoot, 'default/config.yaml'), 'utf8')
const replaceOnce = (from, to) => {
  assert.ok(config.includes(from), `host config anchor missing: ${from}`)
  config = config.replace(from, to)
}
replaceOnce('dataRoot: ./data', `dataRoot: ${JSON.stringify(join(hostRoot, 'data'))}`)
replaceOnce('browserLaunch:\n  # Open the browser automatically on server startup.\n  enabled: true', 'browserLaunch:\n  # Open the browser automatically on server startup.\n  enabled: false')
replaceOnce('port: 8000', `port: ${port}`)
replaceOnce('extensions:\n  # Enable UI extensions\n  enabled: true\n  # Automatically update extensions when a release version changes\n  autoUpdate: true', 'extensions:\n  # Enable UI extensions\n  enabled: true\n  # Automatically update extensions when a release version changes\n  autoUpdate: false')
replaceOnce('    autoDownload: true', '    autoDownload: false')
replaceOnce('enableDownloadableTokenizers: true', 'enableDownloadableTokenizers: false')
replaceOnce('enableServerPluginsAutoUpdate: true', 'enableServerPluginsAutoUpdate: false')
await writeFile(join(hostRoot, 'config.yaml'), config)

console.log(JSON.stringify({
  result: 'PASS',
  hostRoot,
  packageRoot,
  configPath: join(hostRoot, 'config.yaml'),
  version: pinned.version,
  integrity: pinned.integrity,
  port,
}, null, 2))
