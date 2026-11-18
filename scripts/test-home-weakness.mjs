// Run with the learner server and an existing Playwright installation (NODE_PATH).
// API fixtures exercise homepage -> actual grammar detail routing without DB writes.
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { mkdir } from 'node:fs/promises'
const { chromium, expect } = createRequire(import.meta.url)('playwright/test')
const baseURL = process.env.LEARNER_URL || 'http://localhost:35173'
const evidenceDir = process.env.WEAKNESS_EVIDENCE_DIR
const point = { id: 25, name: '〜から（理由）', meaning: '表示原因或理由。', jlpt_level: 'N5', conjunction_rule: '普通形 + から', usage_note: '', examples: [], quiz_questions: [] }
const secondPoint = { ...point, id: 42, name: '〜ながら', meaning: '表示两个动作同时进行。' }
const targetWord = { id: 7, kanji_form: '猫', reading: 'ねこ', meaning: '猫；猫科动物', jlpt_level: 'N4', part_of_speech: '名词', reading_type: '', verb_type: '', examples: [] }
const secondWord = { ...targetWord, id: 8, kanji_form: '鳥', reading: 'とり', meaning: '鸟' }
const queuedWord = { ...targetWord, id: 99, kanji_form: '犬', reading: 'いぬ', jlpt_level: 'N5', meaning: '狗' }
const wordMistake = (id, mode = 'reading') => ({ id, module: 'word', status: 'unmastered', item_key: `word:${id}:${mode}`, source_path: '/words/review', prompt: '错题', wrong_count: 2 })
const mistake = id => ({ id, module: 'grammar', status: 'unmastered', item_key: `grammar:${id}:7`, source_path: '/grammar', prompt: 'テスト問題', wrong_count: 2 })
const panel = page => page.getByRole('heading', { name: '薄弱环节推荐', exact: true }).locator('..').locator('..')
async function followRecommendation(page, expected) {
  const link = panel(page).getByRole('link').filter({ hasText: expected.name })
  await expect(link).toHaveAttribute('href', `/grammar/${expected.id}`)
  if (evidenceDir) {
    await mkdir(evidenceDir, { recursive: true })
    await panel(page).screenshot({ path: `${evidenceDir}/recommendations-${page.viewportSize().width}.png`, animations: 'disabled' })
  }
  await link.click()
  await expect(page).toHaveURL(`${baseURL}/grammar/${expected.id}`)
  await expect(page.getByRole('heading', { name: expected.name, exact: true })).toBeVisible()
  await expect(page.getByText(expected.meaning, { exact: true })).toBeVisible()
}
const cases = [
  { name: '单词推荐直接打开不在今日队列的具体单词，并可评分', wordItems: [wordMistake(7)], items: [mistake(25)], async run(page, state) {
    const link = panel(page).getByRole('link').filter({ hasText: '猫（ねこ）' })
    await expect(link).toHaveAttribute('href', '/words/review?word_id=7')
    await link.click()
    await expect(page.getByText('猫', { exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: 'N4', exact: true })).toHaveAttribute('aria-pressed', 'true')
    assert.equal(state.queueRequests, 0)
    await page.getByPlaceholder('输入假名...', { exact: true }).fill('ねこ')
    await page.getByRole('button', { name: '确认', exact: true }).click()
    await expect(page.getByText(targetWord.meaning, { exact: true })).toBeVisible()
    await page.getByRole('button', { name: '有印象', exact: true }).click()
    await expect(page.getByText('该单词已复习', { exact: true })).toBeVisible()
    assert.deepEqual(state.ratedWords, [7])
  } },
  { name: '具体单词链接刷新后仍定位原词，可返回常规复习', wordItems: [wordMistake(7)], items: [], async run(page, state) {
    await page.goto(`${baseURL}/words/review?word_id=7`)
    await expect(page.getByText('猫', { exact: true })).toBeVisible()
    await page.reload()
    await expect(page.getByText('猫', { exact: true })).toBeVisible()
    assert.equal(state.queueRequests, 0)
    await page.getByRole('button', { name: '返回常规复习', exact: true }).click()
    await expect(page.getByText('犬', { exact: true })).toBeVisible()
    assert.equal(new URL(page.url()).searchParams.has('word_id'), false)
    assert.ok(state.queueRequests > 0)
  } },
  { name: '听写错题直达对应单词的听写练习', wordItems: [wordMistake(7, 'dictation')], items: [], async run(page, state) {
    const link = panel(page).getByRole('link').filter({ hasText: '猫（ねこ）' })
    await expect(link).toHaveAttribute('href', '/words/review?word_id=7&mode=dictation')
    await link.click()
    await expect(page.getByText(targetWord.meaning, { exact: true })).toBeVisible()
    await page.getByRole('textbox', { name: '汉字 / 单词表记', exact: true }).fill('猫')
    await page.getByRole('textbox', { name: '假名读音', exact: true }).fill('ねこ')
    await page.getByRole('button', { name: '确认', exact: true }).click()
    await expect(page.getByRole('heading', { name: '听写正确 ✓', exact: true })).toBeVisible()
    assert.equal(state.queueRequests, 0)
  } },
  { name: '切换认读和听写时保持具体单词', wordItems: [], items: [], async run(page, state) {
    await page.goto(`${baseURL}/words/review?word_id=7`)
    await expect(page.getByText('猫', { exact: true })).toBeVisible()
    await page.getByRole('button', { name: /听写：听读音写单词/ }).click()
    assert.equal(new URL(page.url()).searchParams.get('word_id'), '7')
    await expect(page.getByText(targetWord.meaning, { exact: true })).toBeVisible()
    await page.getByRole('button', { name: /认读：看汉字写假名/ }).click()
    await expect(page.getByText('猫', { exact: true })).toBeVisible()
    assert.equal(state.queueRequests, 0)
  } },
  { name: '没有单词错题时不推荐随机队列', wordItems: [], items: [mistake(25)], async run(page) {
    await expect(panel(page).getByText('暂无待巩固的单词错题', { exact: true })).toBeVisible()
    await expect(panel(page).getByRole('link', { name: /单词复习/ })).toHaveCount(0)
  } },
  { name: '删除的单词会被跳过，推荐下一条有效词条', wordItems: [wordMistake(7), wordMistake(8)], items: [], wordMissing: true, expectedFailure: true, async run(page) {
    const link = panel(page).getByRole('link').filter({ hasText: '鳥（とり）' })
    await expect(link).toHaveAttribute('href', '/words/review?word_id=8')
    await link.click()
    await expect(page.getByText('鳥', { exact: true })).toBeVisible()
  } },
  { name: '失效单词链接明确报错，不误报复习完成', wordItems: [], items: [], wordMissing: true, expectedFailure: true, async run(page) {
    await page.goto(`${baseURL}/words/review?word_id=7`)
    await expect(page.getByRole('alert')).toBeVisible()
    await expect(page.getByText('今日复习完成 🎉', { exact: true })).toHaveCount(0)
    await expect(page.getByText('该单词已复习', { exact: true })).toHaveCount(0)
  } },
  { name: '无效单词 ID 不会加载随机队列，可返回正常复习', wordItems: [], items: [], async run(page, state) {
    await page.goto(`${baseURL}/words/review?word_id=invalid`)
    await expect(page.getByRole('alert')).toContainText('单词链接无效')
    assert.equal(state.queueRequests, 0)
    await page.getByRole('button', { name: '返回常规复习', exact: true }).click()
    await expect(page.getByText('犬', { exact: true })).toBeVisible()
  } },
  { name: '语法推荐使用真实错题和名称，点击直达详情', items: [mistake(25)], async run(page) { await followRecommendation(page, point) } },
  { name: '没有语法错题时不展示虚构的语法推荐', items: [], async run(page) {
    await expect(panel(page).getByText('暂无待巩固的语法错题', { exact: true })).toBeVisible()
    await expect(panel(page).getByText('助词「に / で」的用法区别', { exact: true })).toHaveCount(0)
  } },
  { name: '已删除的语法点跳过后使用下一条有效错题', items: [mistake(25), mistake(42)], missing: true, expectedFailure: true,
    async run(page) { await followRecommendation(page, secondPoint) } },
  { name: '推荐加载失败时保留首页并显示错误', items: [], fail: true, expectedFailure: true, async run(page) {
    await expect(panel(page).getByRole('alert')).toContainText('语法推荐加载失败')
    await expect(page.getByRole('heading', { name: '学习地图', exact: true })).toBeVisible()
    await expect(panel(page).getByText('助词「に / で」的用法区别', { exact: true })).toHaveCount(0)
  } },
  { name: '语法推荐入口始终直达具体语法', items: [mistake(25)], grammarWeakest: true, async run(page) {
    const link = panel(page).getByRole('link').filter({ hasText: point.name })
    await expect(link).toHaveAttribute('href', '/grammar/25')
    await link.click()
    await expect(page.getByRole('heading', { name: point.name, exact: true })).toBeVisible()
    await expect(page).toHaveURL(`${baseURL}/grammar/25`)
  } },
]
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || undefined })
let failures = 0
try {
  for (const width of [1440, 390]) for (const [index, scenario] of cases.entries()) {
    const context = await browser.newContext({ viewport: { width, height: 1000 } })
    const page = await context.newPage()
    page.setDefaultTimeout(5000)
    const errors = []
    const state = { queueRequests: 0, ratedWords: [] }
    page.on('pageerror', err => errors.push(err.message))
    page.on('console', msg => {
      if (msg.type() === 'error' && !(scenario.expectedFailure && (msg.text().startsWith('Failed to load resource') || msg.text().startsWith('[home] failed to load weak')))) errors.push(msg.text())
    })
    await context.addInitScript(() => {
      localStorage.setItem('preferred_language', 'zh')
      localStorage.setItem('token', 'weakness-test-fixture')
      localStorage.setItem('user', JSON.stringify({ id: 1, name: '推荐测试', jlpt_levels: ['N5'] }))
    })
    await page.route('**/api/v1/**', async route => {
      const url = new URL(route.request().url())
      let data
      if (url.pathname.endsWith('/users/stats')) {
        const modules = Object.fromEntries(['word','grammar','speaking','writing','lesson','translation','note'].map(key => [key, { due_count: 1, total_count: 10, mastered_count: (scenario.grammarWeakest ? key === 'grammar' : key === 'word') ? 0 : 5, daily_goal: 3, today_completed: 0 }]))
        data = { streak_days: 1, modules }
      } else if (url.pathname.endsWith('/daily-plan')) data = { date: '2026-01-01', timezone: 'Asia/Shanghai', items: [], total_goal: 0, total_completed: 0, remaining: 0, percent: 0, completed: false }
      else if (url.pathname.endsWith('/mistakes')) {
        if (url.searchParams.get('module') === 'grammar') {
          if (scenario.fail) return route.fulfill({ status: 500, json: { code: 'ERR_INTERNAL', message: 'test failure' } })
          data = { items: scenario.items, total: scenario.items.length, page: 1, size: 10 }
        } else if (url.searchParams.get('module') === 'word') data = { items: scenario.wordItems || [], total: scenario.wordItems?.length || 0, page: 1, size: 10 }
        else data = { items: [{ module: 'word' }], total: 4, page: 1, size: 1 }
      } else if (url.pathname === '/api/v1/words/queue') {
        ++state.queueRequests
        data = [{ word: queuedWord, record: { mastery_level: 2 }, is_new: false }]
      } else if (/\/words\/(7|8)$/.test(url.pathname)) {
        if (url.pathname.endsWith('/7') && scenario.wordMissing) return route.fulfill({ status: 404, json: { code: 'ERR_NOT_FOUND', message: 'deleted word' } })
        data = url.pathname.endsWith('/7') ? targetWord : secondWord
      } else if (/\/words\/\d+\/(rate|seen)$/.test(url.pathname)) {
        if (url.pathname.endsWith('/rate')) state.ratedWords.push(Number(url.pathname.split('/').at(-2)))
        data = { status: 'ok' }
      } else if (url.pathname === '/api/v1/grammar/25') {
        if (scenario.missing) return route.fulfill({ status: 404, json: { code: 'ERR_NOT_FOUND', message: 'deleted' } })
        data = point
      } else if (url.pathname === '/api/v1/grammar/42') data = secondPoint
      else if (url.pathname === '/api/v1/grammar') data = [point, secondPoint]
      else throw new Error(`Unexpected API request: ${url.pathname}`)
      await route.fulfill({ json: { data } })
    })
    try {
      await page.goto(`${baseURL}/`)
      await expect(panel(page)).toBeVisible()
      await scenario.run(page, state)
      await expect(page).toHaveTitle('日本語学習')
      assert.equal(await page.locator('vite-error-overlay').count(), 0)
      assert.deepEqual(errors, [])
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
      console.log(`PASS ${width}px: ${scenario.name}`)
    } catch (err) { ++failures; console.error(`FAIL ${width}px: ${scenario.name}\n${err.message}`) }
    finally {
      if (evidenceDir) {
        await mkdir(evidenceDir, { recursive: true })
        await page.screenshot({ path: `${evidenceDir}/grammar-${width}-${index}.png`, fullPage: true, animations: 'disabled' })
      }
      await context.close()
    }
  }
} finally { await browser.close() }
if (failures) process.exitCode = 1
