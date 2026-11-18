// Run against the learner Vite server: node scripts/test-speaking-recording.mjs
// Requires Playwright (NODE_PATH may point to an existing installation).
// API fixtures keep this regression check from writing to the user's database.
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { mkdir } from 'node:fs/promises'

const { chromium, expect } = createRequire(import.meta.url)('playwright/test')
const baseURL = process.env.LEARNER_URL || 'http://localhost:35173'
const evidenceDir = process.env.SPEAKING_EVIDENCE_DIR
const material = {
  id: 1, type: 'shadow', jlpt_level: 'N5', title: '道を聞く',
  text: 'すみません、駅はどこですか。あの角を右に曲がってください。まっすぐ行くと左側にあります。ありがとうございました。',
}
const savedRecord = { id: 1, material_id: 1, type: 'shadow', score: 80, practiced_at: '2026-01-01T00:00:00Z' }
const nextMaterial = { ...material, id: 4, title: '買い物', text: 'これはいくらですか。' }
const lastMaterial = { ...material, id: 9, title: '自己紹介', text: 'はじめまして。よろしくお願いします。' }
const navigationMaterials = [material, nextMaterial, lastMaterial]
const freeMaterial = { ...material, id: 12, type: 'free', title: '週末', text: '週末は何をしますか。' }
const n4Material = { ...material, id: 14, jlpt_level: 'N4', title: '旅行', text: '旅行に行きたいです。' }
const filterMaterials = [...navigationMaterials, freeMaterial, n4Material]
const filterRecords = [savedRecord, { ...savedRecord, id: 3, material_id: 9 }, { ...savedRecord, id: 4, material_id: 14 }]
const evaluation = {
  status: 'scoreable', overall_score: 80, scores: {}, issues: [], partial: false,
}
const browser = await chromium.launch({
  headless: true,
  executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || undefined,
})
let failures = 0

async function record(page) {
  await page.getByRole('button', { name: /^(开始录音|重新录音)$/ }).click()
  const stop = page.getByRole('button', { name: '停止录音', exact: true })
  await expect(stop).toBeEnabled()
  // Capture enough real MediaRecorder chunks for decoding and playback.
  await page.waitForTimeout(1500)
  await stop.click()
  await expect(page.locator('audio')).toBeVisible()
  await expect.poll(() => page.locator('audio').evaluate(audio => audio.duration)).toBeGreaterThan(0)
  return page.locator('audio').getAttribute('src')
}

const cases = [
  {
    name: '练习状态可在全部、已练习和未练习之间筛选', listView: true, captureFilter: true,
    materials: filterMaterials, records: filterRecords,
    async run(page) {
      const status = page.getByRole('group', { name: '练习状态', exact: true })
      await status.getByRole('button', { name: '已练习', exact: true }).click()
      await expect(page.getByRole('button', { name: /道を聞く/ })).toBeVisible()
      await expect(page.getByRole('button', { name: /自己紹介/ })).toBeVisible()
      await expect(page.getByRole('button', { name: /買い物|週末/ })).toHaveCount(0)
      await status.getByRole('button', { name: '全部', exact: true }).click()
      await expect(page.getByRole('button', { name: /買い物/ })).toBeVisible()
      await expect(page.getByRole('button', { name: /道を聞く/ })).toBeVisible()
      await status.getByRole('button', { name: '未练习', exact: true }).click()
      await expect(page.getByRole('button', { name: /買い物/ })).toBeVisible()
      await expect(page.getByRole('button', { name: /週末/ })).toBeVisible()
      await expect(page.getByRole('button', { name: /道を聞く|自己紹介/ })).toHaveCount(0)
      await expect(status.getByRole('button', { name: '未练习', exact: true })).toHaveAttribute('aria-pressed', 'true')
    },
  },
  {
    name: '练习状态与等级、类型共同筛选，并显示空状态', listView: true,
    materials: filterMaterials, records: filterRecords,
    async run(page) {
      const status = page.getByRole('group', { name: '练习状态', exact: true })
      await status.getByRole('button', { name: '已练习', exact: true }).click()
      await page.getByRole('button', { name: '自由朗读', exact: true }).click()
      await expect(page.getByText('当前筛选下暂无素材', { exact: true })).toBeVisible()
      await status.getByRole('button', { name: '未练习', exact: true }).click()
      await expect(page.getByRole('button', { name: /週末/ })).toBeVisible()
      await expect(page.getByRole('button', { name: /道を聞く|買い物|自己紹介/ })).toHaveCount(0)
      await page.getByRole('button', { name: 'N4', exact: true }).click()
      await page.getByRole('button', { name: '影子跟读', exact: true }).click()
      await status.getByRole('button', { name: '已练习', exact: true }).click()
      await expect(page.getByRole('button', { name: /旅行/ })).toBeVisible()
      await expect(page.getByRole('button', { name: /道を聞く|買い物|自己紹介|週末/ })).toHaveCount(0)
    },
  },
  {
    name: '未练习素材保存后仍可进入下一项，返回列表后更新状态', listView: true,
    materials: filterMaterials, records: filterRecords,
    async run(page) {
      await page.getByRole('group', { name: '练习状态', exact: true }).getByRole('button', { name: '未练习', exact: true }).click()
      await page.getByRole('button', { name: /買い物/ }).click()
      await page.getByRole('button', { name: '☆', exact: true }).last().click()
      await page.getByRole('button', { name: '提交', exact: true }).click()
      await expect(page.getByText('练习已保存！', { exact: true })).toBeVisible()
      await page.getByRole('button', { name: '下一项 →', exact: true }).click()
      await expect(page.getByRole('heading', { name: freeMaterial.title, exact: true })).toBeVisible()
      await expect(page.getByText('第 2 / 2 项', { exact: true })).toBeVisible()
      await expect(page.getByRole('button', { name: '已是最后一项', exact: true })).toBeDisabled()
      await page.getByRole('button', { name: '← 返回列表', exact: true }).click()
      const status = page.getByRole('group', { name: '练习状态', exact: true })
      await expect(status.getByRole('button', { name: '未练习', exact: true })).toHaveAttribute('aria-pressed', 'true')
      await expect(page.getByRole('button', { name: /買い物/ })).toHaveCount(0)
      await expect(page.getByRole('button', { name: /週末/ })).toBeVisible()
      await status.getByRole('button', { name: '已练习', exact: true }).click()
      await expect(page.getByRole('button', { name: /買い物/ })).toBeVisible()
    },
  },
  {
    name: '已练习筛选的下一项跳过未练习素材', listView: true,
    materials: filterMaterials, records: filterRecords,
    async run(page) {
      await page.getByRole('group', { name: '练习状态', exact: true }).getByRole('button', { name: '已练习', exact: true }).click()
      await page.getByRole('button', { name: /道を聞く/ }).click()
      await page.getByRole('button', { name: '下一项 →', exact: true }).click()
      await expect(page.getByRole('heading', { name: lastMaterial.title, exact: true })).toBeVisible()
      await expect(page.getByText('第 2 / 2 项', { exact: true })).toBeVisible()
    },
  },
  {
    name: '记录加载完成前不误判未练习状态', listView: true, holdRecords: true,
    materials: filterMaterials, records: filterRecords,
    async run(page, release) {
      const status = page.getByRole('group', { name: '练习状态', exact: true })
      await expect(status.getByRole('button', { name: '未练习', exact: true })).toBeDisabled()
      await expect(status.getByRole('button', { name: '已练习', exact: true })).toBeDisabled()
      release()
      await expect(status.getByRole('button', { name: '未练习', exact: true })).toBeEnabled()
      await status.getByRole('button', { name: '未练习', exact: true }).click()
      await expect(page.getByRole('button', { name: /道を聞く|自己紹介/ })).toHaveCount(0)
    },
  },
  {
    name: '没有历史记录时素材均为未练习', listView: true, materials: filterMaterials, records: [],
    async run(page) {
      const status = page.getByRole('group', { name: '练习状态', exact: true })
      await status.getByRole('button', { name: '已练习', exact: true }).click()
      await expect(page.getByText('当前筛选下暂无素材', { exact: true })).toBeVisible()
      await status.getByRole('button', { name: '未练习', exact: true }).click()
      await expect(page.getByRole('button', { name: /道を聞く/ })).toBeVisible()
      await expect(page.getByRole('button', { name: /買い物/ })).toBeVisible()
    },
  },
  {
    name: '保存后可直接进入下一项，并清空本次练习状态', materials: navigationMaterials, captureNavigation: true,
    async run(page) {
      await page.getByRole('button', { name: '☆', exact: true }).last().click()
      await page.getByRole('button', { name: '提交', exact: true }).click()
      await expect(page.getByText('练习已保存！', { exact: true })).toBeVisible()
      await page.getByRole('button', { name: '下一项 →', exact: true }).click()
      await expect(page.getByRole('heading', { name: nextMaterial.title, exact: true })).toBeVisible()
      await expect(page.getByText('第 2 / 3 项', { exact: true })).toBeVisible()
      await expect(page.getByText('练习已保存！', { exact: true })).toHaveCount(0)
      await expect(page.getByRole('button', { name: '提交', exact: true })).toBeDisabled()
      await expect(page.getByRole('button', { name: '开始录音', exact: true })).toBeEnabled()
    },
  },
  {
    name: '从列表中间进入也能按顺序前进，到末尾停止', materials: navigationMaterials,
    async run(page) {
      await page.getByRole('button', { name: '← 返回列表', exact: true }).click()
      await page.getByRole('button', { name: /買い物/ }).click()
      await page.getByRole('button', { name: '下一项 →', exact: true }).click()
      await expect(page.getByRole('heading', { name: lastMaterial.title, exact: true })).toBeVisible()
      await expect(page.getByText('第 3 / 3 项', { exact: true })).toBeVisible()
      await expect(page.getByRole('button', { name: '已是最后一项', exact: true })).toBeDisabled()
      await expect(page.getByRole('button', { name: '← 返回列表', exact: true })).toBeEnabled()
    },
  },
  {
    name: '保存请求完成前禁用切换，完成后恢复下一项', materials: navigationMaterials, holdSubmission: true,
    async run(page, release) {
      await page.getByRole('button', { name: '☆', exact: true }).last().click()
      await page.getByRole('button', { name: '提交', exact: true }).click()
      await expect(page.getByRole('button', { name: '下一项 →', exact: true })).toBeDisabled()
      await expect(page.getByRole('button', { name: '← 返回列表', exact: true })).toBeDisabled()
      release()
      await expect(page.getByText('练习已保存！', { exact: true })).toBeVisible()
      await expect(page.getByRole('button', { name: '下一项 →', exact: true })).toBeEnabled()
    },
  },
  {
    name: '录音中切换下一项会释放麦克风', materials: navigationMaterials,
    async run(page) {
      await page.getByRole('button', { name: '开始录音', exact: true }).click()
      await expect(page.getByRole('button', { name: '停止录音', exact: true })).toBeEnabled()
      await page.getByRole('button', { name: '下一项 →', exact: true }).click()
      await expect(page.getByRole('heading', { name: nextMaterial.title, exact: true })).toBeVisible()
      await expect(page.getByRole('button', { name: '开始录音', exact: true })).toBeEnabled()
      assert.equal(await page.evaluate(() => window.captureStreams.every(stream => stream.getTracks().every(track => track.readyState === 'ended'))), true)
      await expect(page.locator('audio')).toHaveCount(0)
    },
  },
  {
    name: '评分中切换下一项不会带入旧回放和迟到的评分', materials: navigationMaterials, holdEvaluation: true,
    async run(page, release) {
      await record(page)
      await expect(page.getByRole('status').filter({ hasText: '正在评分' })).toBeVisible()
      await page.getByRole('button', { name: '下一项 →', exact: true }).click()
      await expect(page.getByRole('heading', { name: nextMaterial.title, exact: true })).toBeVisible()
      release()
      await expect(page.locator('audio')).toHaveCount(0)
      await expect(page.getByLabel('本次跟读评分')).toHaveCount(0)
      await expect(page.getByRole('button', { name: '开始录音', exact: true })).toBeEnabled()
    },
  },
  {
    name: '已练习素材可再次录音、回放和保存',
    async run(page) {
      const firstURL = await record(page)
      await expect(page.getByRole('button', { name: '重新录音', exact: true })).toBeEnabled()
      const nextURL = await record(page)
      assert.notEqual(nextURL, firstURL)
      await page.locator('audio').evaluate(audio => audio.play())
      await expect.poll(() => page.locator('audio').evaluate(audio => audio.currentTime)).toBeGreaterThan(0)
      await page.getByRole('button', { name: '提交', exact: true }).click()
      await expect(page.getByText('练习已保存！', { exact: true })).toBeVisible()
      await page.getByRole('button', { name: '重新录音', exact: true }).click()
      await expect(page.getByRole('button', { name: '停止录音', exact: true })).toBeEnabled()
      await expect(page.getByText('练习已保存！', { exact: true })).toHaveCount(0)
    },
  },
  {
    name: '评分等待中可以重新录音，旧评分不会覆盖新练习', holdEvaluation: true,
    async run(page, release) {
      await record(page)
      await expect(page.getByRole('status').filter({ hasText: '正在评分' })).toBeVisible()
      await expect(page.getByRole('button', { name: '重新录音', exact: true })).toBeEnabled()
      await page.getByRole('button', { name: '重新录音', exact: true }).click()
      await expect(page.getByRole('button', { name: '停止录音', exact: true })).toBeEnabled()
      release()
      await expect(page.getByRole('status').filter({ hasText: '正在评分' })).toHaveCount(0)
      await expect(page.getByLabel('本次跟读评分')).toHaveCount(0)
    },
  },
  {
    name: '重新录音失败后保留旧回放，并可恢复录音', capturePlayback: true,
    async run(page) {
      const url = await record(page)
      await page.evaluate(() => { window.failNextCapture = true })
      await page.getByRole('button', { name: '重新录音', exact: true }).click()
      await expect(page.getByText('Microphone temporarily unavailable', { exact: true })).toBeVisible()
      await expect(page.locator('audio')).toHaveAttribute('src', url)
      assert.equal(await page.evaluate(async url => (await fetch(url)).ok, url), true)
      await expect(page.getByRole('button', { name: '重新录音', exact: true })).toBeEnabled()
      await record(page)
      await expect(page.getByText('Microphone temporarily unavailable', { exact: true })).toHaveCount(0)
    },
  },
  {
    name: '首次麦克风失败后可直接重试',
    async run(page) {
      await page.evaluate(() => { window.failNextCapture = true })
      await page.getByRole('button', { name: '开始录音', exact: true }).click()
      await expect(page.getByText('Microphone temporarily unavailable', { exact: true })).toBeVisible()
      await expect(page.getByRole('button', { name: '开始录音', exact: true })).toBeEnabled()
      await record(page)
    },
  },
]

try {
  for (const viewport of [{ width: 1440, height: 1100 }, { width: 390, height: 844 }]) {
    for (const [index, scenario] of cases.entries()) {
      const context = await browser.newContext({ viewport })
      const page = await context.newPage()
      page.setDefaultTimeout(5000)
      const errors = []
      page.on('pageerror', err => errors.push(err.message))
      page.on('console', msg => { if (msg.type() === 'error') errors.push(msg.text()) })
      let release
      const evaluationGate = new Promise(resolve => { release = resolve })
      await context.addInitScript(() => {
        localStorage.setItem('preferred_language', 'zh')
        localStorage.setItem('token', 'speaking-regression-fixture')
        localStorage.setItem('user', JSON.stringify({ id: 1, name: '录音测试' }))
        window.captureStreams = []
        // A deterministic Web Audio source exercises the real MediaRecorder,
        // decoder, WAV encoder and <audio> playback without physical hardware.
        navigator.mediaDevices.getUserMedia = async () => {
          if (window.failNextCapture) {
            window.failNextCapture = false
            throw new Error('Microphone temporarily unavailable')
          }
          const context = new AudioContext()
          const source = context.createOscillator()
          const destination = context.createMediaStreamDestination()
          source.connect(destination)
          source.start()
          await context.resume()
          window.captureStreams.push(destination.stream)
          return destination.stream
        }
      })
      await page.route('**/api/v1/**', async route => {
        const requestURL = new URL(route.request().url())
        const path = requestURL.pathname
        let data
        if (path.endsWith('/materials')) data = (scenario.materials || [material]).filter(item =>
          (!requestURL.searchParams.get('level') || item.jlpt_level === requestURL.searchParams.get('level')) &&
          (!requestURL.searchParams.get('type') || item.type === requestURL.searchParams.get('type')))
        else if (path.endsWith('/records')) {
          if (scenario.holdRecords) await evaluationGate
          data = scenario.records || [savedRecord]
        }
        else if (path.endsWith('/stats')) data = { modules: { speaking: { today_completed: 0, daily_goal: 3 } } }
        else if (path.endsWith('/practice')) {
          if (scenario.holdSubmission) await evaluationGate
          data = { ...savedRecord, ...route.request().postDataJSON(), id: 100 }
        }
        else if (path.endsWith('/evaluate')) {
          if (scenario.holdEvaluation) await evaluationGate
          data = evaluation
        } else throw new Error(`Unexpected API request: ${path}`)
        await route.fulfill({ json: { data } }).catch(err => {
          // A replaced recording aborts its outstanding scoring request.
          if (!scenario.holdEvaluation && !scenario.holdRecords) throw err
        })
      })
      try {
        await page.goto(`${baseURL}/speaking`)
        assert.equal(new URL(page.url()).pathname, '/speaking')
        await expect(page).toHaveTitle('日本語学習')
        await expect(page.getByRole('heading', { name: '口语练习', exact: true })).toBeVisible()
        if (!scenario.listView) {
          await page.getByRole('button', { name: /道を聞く/ }).click()
          if (!scenario.materials) await expect(page.getByRole('button', { name: '已是最后一项', exact: true })).toBeDisabled()
        }
        await scenario.run(page, release)
        await expect(page.getByRole('status').filter({ hasText: '正在评分' })).toHaveCount(0)
        assert.equal(await page.locator('vite-error-overlay').count(), 0)
        assert.deepEqual(errors, [])
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
        if (evidenceDir && scenario.captureFilter) {
          await mkdir(evidenceDir, { recursive: true })
          await page.getByRole('group', { name: '练习状态', exact: true }).screenshot({ path: `${evidenceDir}/speaking-filter-${viewport.width}.png`, animations: 'disabled' })
        }
        if (evidenceDir && scenario.captureNavigation) {
          await mkdir(evidenceDir, { recursive: true })
          await page.getByRole('button', { name: '← 返回列表', exact: true }).locator('..').screenshot({ path: `${evidenceDir}/speaking-next-${viewport.width}.png` })
        }
        if (evidenceDir && scenario.capturePlayback && viewport.width >= 1024) {
          await mkdir(evidenceDir, { recursive: true })
          await page.locator('audio').evaluate(audio => audio.scrollIntoView({ block: 'center' }))
          const toolbar = await page.getByRole('button', { name: '播放参考', exact: true }).locator('..').boundingBox()
          const playback = await page.locator('audio').boundingBox()
          await page.screenshot({
            path: `${evidenceDir}/speaking-retry-controls-${viewport.width}.png`,
            clip: { x: toolbar.x, y: toolbar.y, width: toolbar.width, height: playback.y + playback.height - toolbar.y },
          })
        }
        console.log(`PASS ${viewport.width}px: ${scenario.name}`)
      } catch (err) {
        ++failures
        console.error(`FAIL ${viewport.width}px: ${scenario.name}\n${err.message}`)
      } finally {
        release()
        if (evidenceDir) {
          await mkdir(evidenceDir, { recursive: true })
          await page.screenshot({ path: `${evidenceDir}/speaking-${viewport.width}-${index}.png`, fullPage: true })
        }
        await context.close()
      }
    }
  }
} finally {
  await browser.close()
}
if (failures) process.exitCode = 1
