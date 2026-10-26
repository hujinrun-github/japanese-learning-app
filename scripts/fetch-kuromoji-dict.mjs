#!/usr/bin/env node
/**
 * Download the kuromoji.js browser dictionary into the learner frontend's public dir.
 *
 * The dictionary (~19 MB of .gz files) is NOT tracked in git. It is fetched at
 * deploy time (see .github/workflows/deploy.yml) and on demand by `npm run prepare:dict`.
 *
 * Strategy, in order:
 *   1. npm tarball of the exact kuromoji version we depend on (single request, authoritative)
 *   2. per-file download from a CDN (unpkg, then jsdelivr)
 *
 * kuromoji.js only ever requests the gzipped files listed below, so the
 * uncompressed .dat copies (113 MB) that used to live in the repo are dead weight
 * and are pruned when present.
 *
 * Idempotent: if every file is already present and non-empty, nothing is downloaded.
 */
import { execFile } from 'node:child_process'
import { mkdir, mkdtemp, readdir, rename, rm, stat, writeFile, copyFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'

const execFileAsync = promisify(execFile)

const KUROMOJI_VERSION = '0.1.2'
const TARBALL_URL = `https://registry.npmjs.org/kuromoji/-/kuromoji-${KUROMOJI_VERSION}.tgz`

const DICT_FILES = [
  'base.dat.gz',
  'cc.dat.gz',
  'check.dat.gz',
  'tid.dat.gz',
  'tid_map.dat.gz',
  'tid_pos.dat.gz',
  'unk.dat.gz',
  'unk_char.dat.gz',
  'unk_compat.dat.gz',
  'unk_invoke.dat.gz',
  'unk_map.dat.gz',
  'unk_pos.dat.gz',
]

const CDN_HOSTS = [
  `https://unpkg.com/kuromoji@${KUROMOJI_VERSION}/dict`,
  `https://cdn.jsdelivr.net/npm/kuromoji@${KUROMOJI_VERSION}/dict`,
]

const scriptDir = dirname(fileURLToPath(import.meta.url))
const repoRoot = resolve(scriptDir, '..')
const targetDir = join(repoRoot, 'front', 'react', 'public', 'kuromoji-dict')

function log(message) {
  console.log(`[kuromoji-dict] ${message}`)
}

async function sizeOf(path) {
  try {
    return (await stat(path)).size
  } catch {
    return -1
  }
}

async function missingFiles() {
  const missing = []
  for (const name of DICT_FILES) {
    if ((await sizeOf(join(targetDir, name))) <= 0) {
      missing.push(name)
    }
  }
  return missing
}

async function fetchToBuffer(url) {
  const response = await fetch(url, { redirect: 'follow' })
  if (!response.ok) {
    throw new Error(`HTTP ${response.status}`)
  }
  const bytes = Buffer.from(await response.arrayBuffer())
  if (bytes.length === 0) {
    throw new Error('empty body')
  }
  return bytes
}

async function installFromTarball(missing) {
  const workDir = await mkdtemp(join(tmpdir(), 'kuromoji-dict-'))
  const tarball = join(workDir, 'kuromoji.tgz')
  try {
    log(`fetch ${TARBALL_URL}`)
    await writeFile(tarball, await fetchToBuffer(TARBALL_URL))
    await execFileAsync('tar', ['-xzf', tarball, '-C', workDir, '--strip-components=2', 'package/dict'])
    for (const name of missing) {
      const source = join(workDir, name)
      if ((await sizeOf(source)) <= 0) {
        throw new Error(`tarball is missing ${name}`)
      }
      await copyFile(source, join(targetDir, name))
      log(`saved ${name}`)
    }
    return true
  } catch (error) {
    log(`tarball install failed: ${error.message}`)
    return false
  } finally {
    await rm(workDir, { recursive: true, force: true })
  }
}

async function installFromCdn(missing) {
  for (const host of CDN_HOSTS) {
    let ok = true
    for (const name of missing) {
      try {
        const bytes = await fetchToBuffer(`${host}/${name}`)
        // Write a temp file first so an interrupted run never leaves a truncated dict.
        await writeFile(`${join(targetDir, name)}.part`, bytes)
        await rename(`${join(targetDir, name)}.part`, join(targetDir, name))
        log(`saved ${name} from ${host}`)
      } catch (error) {
        log(`failed ${host}/${name}: ${error.message}`)
        ok = false
        break
      }
    }
    if (ok) {
      return true
    }
  }
  return false
}

/**
 * Remove uncompressed .dat copies that kuromoji.js never requests.
 * Only files that have a .gz counterpart are pruned.
 */
async function pruneUncompressed() {
  let entries
  try {
    entries = await readdir(targetDir)
  } catch {
    return 0
  }
  let pruned = 0
  for (const entry of entries) {
    if (!entry.endsWith('.dat') || !DICT_FILES.includes(`${entry}.gz`)) {
      continue
    }
    await rm(join(targetDir, entry), { force: true })
    pruned++
  }
  return pruned
}

async function main() {
  await mkdir(targetDir, { recursive: true })

  const missing = await missingFiles()
  if (missing.length === 0) {
    log(`dictionary up to date at ${targetDir}`)
  } else {
    log(`fetching ${missing.length} file(s) into ${targetDir}`)
    const installed = (await installFromTarball(missing)) || (await installFromCdn(missing))
    if (!installed) {
      throw new Error('every download source failed')
    }
  }

  const pruned = await pruneUncompressed()
  if (pruned > 0) {
    log(`pruned ${pruned} uncompressed .dat file(s) (browser only needs .gz)`)
  }

  const stillMissing = await missingFiles()
  if (stillMissing.length > 0) {
    throw new Error(`dictionary incomplete, missing: ${stillMissing.join(', ')}`)
  }

  let total = 0
  for (const name of DICT_FILES) {
    total += await sizeOf(join(targetDir, name))
  }
  log(`dictionary ready (${(total / 1024 / 1024).toFixed(1)} MB)`)
}

main().catch(error => {
  console.error(`[kuromoji-dict] ERROR: ${error.message}`)
  process.exit(1)
})
