// Learner Vite server + existing Playwright installation (optionally via NODE_PATH).
// All APIs use fixtures; browser navigation/reloads exercise real React state.
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { mkdir } from 'node:fs/promises'
const { chromium, expect } = createRequire(import.meta.url)('playwright/test')
const baseURL = process.env.LEARNER_URL || 'http://localhost:35173'
const evidenceDir = process.env.WRITING_EVIDENCE_DIR
const topics = ['weekend', 'favorite-place', 'trip-plan'].map((id, i) => ({
  id, title: ['我的周末', '我喜欢的一个地方', '旅行计划'][i], jlpt_level: ['N5', 'N5', 'N4'][i],
  essay_prompt: '用日语写一篇短文。', sentence_prompt: '用「〜から」写一个完整的句子。',
}))
const answer = page => page.getByRole('textbox', { name: '你的日语回答', exact: true })
const next = page => page.getByRole('button', { name: '下一个主题', exact: true }).first()
const level = page => page.getByRole('combobox', { name: '级别', exact: true })
const status = page => page.getByRole('combobox', { name: '练习状态', exact: true })
const save = async page => {
  await page.getByRole('button', { name: '保存草稿', exact: true }).click()
  await expect(page.getByText('草稿已保存', { exact: true })).toBeVisible()
}
const cases = [
  { name: '加载失败时筛选保持禁用，重试成功后恢复', regression: true, expectedFailure: true, setup(state) {
    state.failLoad = true
  }, async run(page, state) {
    await expect(page.getByRole('alert')).toBeVisible()
    await expect(level(page)).toBeDisabled()
    await expect(status(page)).toBeDisabled()
    await expect(page.getByRole('button', { name: '重新加载', exact: true })).toBeVisible()
    state.failLoad = false
    await page.getByRole('button', { name: '重新加载', exact: true }).click()
    await expect(page.getByRole('heading', { name: topics[0].title, exact: true })).toBeVisible()
    await expect(level(page)).toBeEnabled()
    await expect(status(page)).toBeEnabled()
    await expect(page.getByRole('alert')).toHaveCount(0)
  } },
  { name: '级别与练习状态可以组合筛选，默认不重复旧主题', regression: true, setup(state) {
    state.practiced.add('1:weekend')
  }, async run(page) {
    await expect(status(page)).toHaveValue('unpracticed')
    await expect(page.getByRole('heading', { name: topics[1].title, exact: true })).toBeVisible()
    await level(page).selectOption('N4')
    await expect(page.getByRole('heading', { name: topics[2].title, exact: true })).toBeVisible()
    await status(page).selectOption('practiced')
    await expect(answer(page)).toHaveCount(0)
    await level(page).selectOption('N5')
    await expect(page.getByRole('heading', { name: topics[0].title, exact: true })).toBeVisible()
    await expect(page.locator('section').getByText('已练习', { exact: true })).toBeVisible()
    await status(page).selectOption('all')
    await next(page).click()
    await expect(page.getByRole('heading', { name: topics[1].title, exact: true })).toBeVisible()
  } },
  { name: '没有匹配级别时显示空状态，改回筛选可继续练习', regression: true, async run(page) {
    await level(page).selectOption('N2')
    await expect(answer(page)).toHaveCount(0)
    await expect(page.getByRole('button', { name: '提交', exact: true })).toHaveCount(0)
    await expect(page.getByText(/没有符合筛选条件的主题/)).toBeVisible()
    await level(page).selectOption('N4')
    await expect(page.getByRole('heading', { name: topics[2].title, exact: true })).toBeVisible()
  } },
  { name: '成功提交后刷新会排除旧题，已练习筛选仍可查看', regression: true, async run(page, state) {
    await answer(page).fill('週末は本を読みました。')
    await page.getByRole('button', { name: '提交', exact: true }).click()
    await expect(page.getByText('已保存回答，当前未启用 AI 批改。', { exact: true })).toBeVisible()
    assert.equal(state.practiced.has('1:weekend'), true)
    await page.reload()
    await expect(page.getByRole('heading', { name: topics[1].title, exact: true })).toBeVisible()
    await status(page).selectOption('practiced')
    await expect(page.getByRole('heading', { name: topics[0].title, exact: true })).toBeVisible()
  } },
  { name: '成功提交后下一主题不会漏过未练习题', regression: true, async run(page) {
    await answer(page).fill('週末は図書館に行きました。')
    await page.getByRole('button', { name: '提交', exact: true }).click()
    await expect(page.getByText('已保存回答，当前未启用 AI 批改。', { exact: true })).toBeVisible()
    await next(page).click()
    await expect(page.getByRole('heading', { name: topics[1].title, exact: true })).toBeVisible()
    await next(page).click()
    await expect(page.getByRole('heading', { name: topics[2].title, exact: true })).toBeVisible()
  } },
  { name: '所有主题练习过时默认无题，主动切换全部才显示旧题', regression: true, setup(state) {
    for (const topic of topics) state.practiced.add(`1:${topic.id}`)
  }, async run(page) {
    await expect(answer(page)).toHaveCount(0)
    await expect(page.getByText(/没有符合筛选条件的主题/)).toBeVisible()
    await status(page).selectOption('all')
    await expect(page.getByRole('heading', { name: topics[0].title, exact: true })).toBeVisible()
  } },
  { name: '切换全部主题后能访问当前题之前的已练习题', regression: true, setup(state) {
    state.practiced.add('1:weekend')
  }, async run(page) {
    await expect(page.getByRole('heading', { name: topics[1].title, exact: true })).toBeVisible()
    await status(page).selectOption('all')
    await expect(page.getByRole('heading', { name: topics[0].title, exact: true })).toBeVisible()
  } },
  { name: '切换筛选先保存草稿，失败时保留原筛选和文字', regression: true, expectedFailure: true, async run(page, state) {
    await answer(page).fill('筛选前的下書き。')
    state.failSave = true
    await level(page).selectOption('N4')
    await expect(page.getByRole('alert')).toContainText('草稿保存失败')
    await expect(level(page)).toHaveValue('all')
    await expect(answer(page)).toHaveValue('筛选前的下書き。')
    state.failSave = false
    await level(page).selectOption('N4')
    await expect(page.getByRole('heading', { name: topics[2].title, exact: true })).toBeVisible()
    assert.equal(state.drafts.get('1:weekend:essay').user_answer, '筛选前的下書き。')
    await level(page).selectOption('all')
    await expect(answer(page)).toHaveValue('筛选前的下書き。')
  } },
  { name: '练习状态按账号隔离', regression: true, setup(state) { state.practiced.add('1:weekend') }, async run(page) {
    await expect(page.getByRole('heading', { name: topics[1].title, exact: true })).toBeVisible()
    await page.evaluate(() => {
      localStorage.setItem('token', 'writing-test-2')
      localStorage.setItem('user', JSON.stringify({ id: 2, name: '写作测试二' }))
    })
    await page.reload()
    await expect(page.getByRole('heading', { name: topics[0].title, exact: true })).toBeVisible()
  } },
  { name: '两种模式的草稿保存后刷新仍可继续', evidence: true, async run(page) {
    await answer(page).fill('土曜日は図書館に行きました。\n本を読みました。')
    await save(page)
    await page.getByRole('button', { name: '情境造句', exact: true }).click()
    await answer(page).fill('天気がいいですから、散歩します。')
    await save(page)
    await page.reload()
    await expect(answer(page)).toHaveValue('天気がいいですから、散歩します。')
    await expect(page.getByRole('button', { name: '情境造句', exact: true })).toHaveAttribute('aria-pressed', 'true')
    await page.getByRole('button', { name: '主题短文', exact: true }).click()
    await expect(answer(page)).toHaveValue('土曜日は図書館に行きました。\n本を読みました。')
  } },
  { name: '未提交即可跳下一项，自动保存并恢复后续题目', async run(page, state) {
    await answer(page).fill('週末は映画を見ました。')
    await next(page).click()
    await expect(page.getByRole('heading', { name: topics[1].title, exact: true })).toBeVisible()
    assert.equal(state.drafts.get('1:weekend:essay').user_answer, '週末は映画を見ました。')
    await expect(answer(page)).toHaveValue('')
    await answer(page).fill('図書館が好きです。')
    await save(page)
    await page.goto(`${baseURL}/speaking`)
    await page.goto(`${baseURL}/writing`)
    await expect(page.getByRole('heading', { name: topics[1].title, exact: true })).toBeVisible()
    await expect(answer(page)).toHaveValue('図書館が好きです。')
  } },
  { name: '保存失败不会跳题或丢失内容，重试可恢复', expectedFailure: true, async run(page, state) {
    await answer(page).fill('下書きを失わないでください。')
    state.failSave = true
    await next(page).click()
    await expect(page.getByRole('alert')).toContainText('草稿保存失败')
    await expect(page.getByRole('heading', { name: topics[0].title, exact: true })).toBeVisible()
    await expect(answer(page)).toHaveValue('下書きを失わないでください。')
    state.failSave = false
    await save(page)
    await page.reload()
    await expect(answer(page)).toHaveValue('下書きを失わないでください。')
  } },
  { name: '最后一项不会越界，提交后仍能完成练习', async run(page) {
    await next(page).click()
    await next(page).click()
    await expect(page.getByRole('button', { name: '已是最后一项', exact: true })).toBeDisabled()
    await answer(page).fill('旅行に行きたいです。')
    await page.getByRole('button', { name: '提交', exact: true }).click()
    await page.getByRole('button', { name: '完成练习', exact: true }).first().click()
    await expect(page.getByText('全部完成 🎉', { exact: true })).toBeVisible()
  } },
  { name: '成功提交仅清理当前模式，其他草稿继续保留', async run(page, state) {
    await answer(page).fill('短文の下書きです。')
    await page.getByRole('button', { name: '情境造句', exact: true }).click()
    await answer(page).fill('今日は休みですから、散歩します。')
    await save(page)
    await page.getByRole('button', { name: '提交', exact: true }).click()
    await expect(page.getByText('已保存回答，当前未启用 AI 批改。', { exact: true })).toBeVisible()
    assert.equal(state.drafts.has('1:weekend:sentence'), false)
    assert.equal(state.drafts.get('1:weekend:essay').user_answer, '短文の下書きです。')
    await page.reload()
    await status(page).selectOption('all')
    await expect(answer(page)).toHaveValue('短文の下書きです。')
  } },
  { name: '不同账号的草稿相互隔离', async run(page) {
    await answer(page).fill('一人目の下書き。')
    await save(page)
    await page.evaluate(() => {
      localStorage.setItem('token', 'writing-test-2')
      localStorage.setItem('user', JSON.stringify({ id: 2, name: '写作测试二' }))
    })
    await page.reload()
    await expect(answer(page)).toHaveValue('')
  } },
  { name: '保存期间禁用输入和跳题，完成后恢复', async run(page, state) {
    let release
    state.saveGate = new Promise(resolve => { release = resolve })
    await answer(page).fill('保存中の下書き。')
    await page.getByRole('button', { name: '保存草稿', exact: true }).click()
    await expect(answer(page)).toBeDisabled()
    await expect(next(page)).toBeDisabled()
    await expect(page.getByRole('button', { name: '提交', exact: true })).toBeDisabled()
    release()
    await expect(page.getByText('草稿已保存', { exact: true })).toBeVisible()
    await expect(answer(page)).toBeEnabled()
    await expect(next(page)).toBeEnabled()
  } },
  { name: '清空已保存的草稿后重新进入不再恢复旧文字', async run(page) {
    await answer(page).fill('消す予定の下書き。')
    await save(page)
    await answer(page).fill('')
    await save(page)
    await page.reload()
    await expect(answer(page)).toHaveValue('')
  } },
  { name: '提交失败时已保存草稿仍可刷新接续', expectedFailure: true, async run(page, state) {
    state.failSubmit = true
    await answer(page).fill('提出に失敗しても残してください。')
    await page.getByRole('button', { name: '提交', exact: true }).click()
    await expect(page.getByRole('alert')).toBeVisible()
    await expect(answer(page)).toHaveValue('提出に失敗しても残してください。')
    await page.reload()
    await expect(answer(page)).toHaveValue('提出に失敗しても残してください。')
  } },
  { name: '超出提交字数的未完成内容也可保存为草稿', async run(page) {
    await answer(page).fill('あ'.repeat(1001))
    await expect(page.getByRole('button', { name: '提交', exact: true })).toBeDisabled()
    await save(page)
    await page.reload()
    await expect(answer(page)).toHaveValue('あ'.repeat(1001))
  } },
  { name: '恢复已保存版本并成功提交后清除旧的保存失败提示', expectedFailure: true, async run(page, state) {
    await answer(page).fill('保存した文章です。')
    await save(page)
    await answer(page).fill('新しい文章です。')
    state.failSave = true
    await next(page).click()
    await expect(page.getByRole('alert')).toContainText('草稿保存失败')
    state.failSave = false
    await answer(page).fill('保存した文章です。')
    await page.getByRole('button', { name: '提交', exact: true }).click()
    await expect(page.getByText('已保存回答，当前未启用 AI 批改。', { exact: true })).toBeVisible()
    await expect(page.getByRole('alert')).toHaveCount(0)
  } },
]
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || undefined })
let failures = 0
const warnings = new Set()
try {
  for (const width of [1440, 390]) for (const [index, scenario] of cases.entries()) {
    if (process.env.WRITING_REGRESSION_ONLY && !scenario.regression) continue
    if (process.env.WRITING_SCENARIO_FILTER && !scenario.name.includes(process.env.WRITING_SCENARIO_FILTER)) continue
    const context = await browser.newContext({ viewport: { width, height: 1000 } })
    const page = await context.newPage()
    page.setDefaultTimeout(5000)
    const state = { drafts: new Map(), practiced: new Set(), failSave: false, sequence: 0 }
    scenario.setup?.(state)
    const errors = []
    page.on('pageerror', err => errors.push(err.message))
    page.on('console', msg => {
      if (msg.type() === 'error' && !(scenario.expectedFailure && msg.text().startsWith('Failed to load resource'))) errors.push(msg.text())
      if (msg.type() === 'warning') warnings.add(msg.text())
    })
    await context.addInitScript(() => {
      localStorage.setItem('preferred_language', 'zh')
      if (!localStorage.getItem('token')) {
        localStorage.setItem('token', 'writing-test-1')
        localStorage.setItem('user', JSON.stringify({ id: 1, name: '写作测试' }))
      }
    })
    await page.route('**/api/v1/**', async route => {
      const request = route.request()
      const path = new URL(request.url()).pathname
      const userID = request.headers().authorization?.endsWith('-2') ? 2 : 1
      let data
      if (path.endsWith('/writing/topics')) {
        if (state.failLoad) return route.fulfill({ status: 500, json: { code: 'ERR_INTERNAL', message: 'load failed' } })
        data = topics.map(topic => ({ ...topic, practiced: state.practiced.has(`${userID}:${topic.id}`) }))
      }
      else if (path.endsWith('/writing/drafts')) data = [...state.drafts.values()].filter(draft => draft.user_id === userID).sort((a,b) => b.sequence-a.sequence)
      else if (path.endsWith('/stats')) data = { modules: { writing: { today_completed: 0, daily_goal: 3 }, speaking: { today_completed: 0, daily_goal: 3 } } }
      else if (path.endsWith('/records') || path.endsWith('/materials')) data = []
      else if (path.endsWith('/draft')) {
        if (state.saveGate) await state.saveGate
        if (state.failSave) return route.fulfill({ status: 500, json: { code: 'ERR_INTERNAL', message: 'save failed' } })
        const body = request.postDataJSON()
        const topicID = path.split('/').at(-2)
        const key = `${userID}:${topicID}:${body.type}`
        data = { ...body, topic_id: topicID, user_id: userID, updated_at: new Date().toISOString(), sequence: ++state.sequence }
        if (body.user_answer.trim()) state.drafts.set(key, data)
        else state.drafts.delete(key)
      } else if (request.method() === 'POST' && path.includes('/writing/topics/')) {
        if (state.failSubmit) return route.fulfill({ status: 500, json: { code: 'ERR_INTERNAL', message: 'submit failed' } })
        const body = request.postDataJSON()
        state.practiced.add(`${userID}:${path.split('/').at(-1)}`)
        state.drafts.delete(`${userID}:${path.split('/').at(-1)}:${body.type}`)
        data = { id: 1, ...body, score: 0, question: '写作题目', practiced_at: new Date().toISOString() }
      } else throw new Error(`Unexpected API: ${path}`)
      await route.fulfill({ json: { data } })
    })
    try {
      await page.goto(`${baseURL}/writing`)
      await expect(page.getByRole('heading', { name: '写作练习', exact: true })).toBeVisible()
      await scenario.run(page, state)
      if (index === 0) {
        if (width > 600) {
          await expect(page.locator('header').getByRole('link', { name: '写作', exact: true })).toHaveAttribute('href', '/writing')
        } else {
          await page.getByRole('navigation', { name: '主导航' }).getByRole('button', { name: /更多/ }).click()
          const more = page.getByRole('dialog', { name: '更多', exact: true })
          await expect(more.getByRole('link', { name: /写作/ })).toHaveAttribute('href', '/writing')
          await more.getByRole('button', { name: '关闭菜单', exact: true }).click()
        }
        await page.getByRole('button', { name: '账号菜单', exact: true }).click()
        await expect(page.getByRole('button', { name: /🏠 首页/ })).toBeVisible()
        await page.getByRole('button', { name: '账号菜单', exact: true }).click()
      }
      assert.equal(new URL(page.url()).pathname, '/writing')
      await expect(page).toHaveTitle('日本語学習')
      assert.equal(await page.locator('vite-error-overlay').count(), 0)
      assert.deepEqual(errors, [])
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
      console.log(`PASS ${width}px: ${scenario.name}`)
    } catch (err) { ++failures; console.error(`FAIL ${width}px: ${scenario.name}\n${err.message}`) }
    finally {
      if (evidenceDir) {
        await mkdir(evidenceDir, { recursive: true })
        await page.screenshot({ path: `${evidenceDir}/writing-${width}-${index}.png`, fullPage: true, animations: 'disabled' })
      }
      await context.close()
    }
  }
} finally { await browser.close() }
if (warnings.size) console.log(`Console warnings: ${JSON.stringify([...warnings])}`)
if (failures) process.exitCode = 1
