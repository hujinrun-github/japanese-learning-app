import { describe, expect, it } from 'vitest'
import { errataSourcePath } from './errataSource'

// item_key 由各练习页的「勘误」按钮采集，形态与后端 rules.go 的 module 规则一致。
// 这里验证：历史报告无需后端新增字段，也能还原出可点击的原题站内路径。
describe('errataSourcePath 按模块与定位键还原原题路径', () => {
  it.each([
    { name: '语法点（例句 / 音频类）直达例句区', module: 'grammar', item_key: '12', expected: '/grammar/12#examples' },
    { name: '语法测验题自动展开并定位到题目', module: 'grammar', item_key: '12:quiz:34', expected: '/grammar/12?quiz=1#quiz-34' },
    { name: '语法例句直达具体原句', module: 'grammar', item_key: '12:example:34', expected: '/grammar/12#example-34' },
    { name: '语法定位键后缀异常时仍回语法详情', module: 'grammar', item_key: '12:quiz', expected: '/grammar/12#examples' },
    { name: '单词认读题定位到原词', module: 'word', item_key: '56', expected: '/words/review?word_id=56' },
    { name: '单词认读题保留模式', module: 'word', item_key: '56:recognition', expected: '/words/review?word_id=56' },
    { name: '单词听写题保留听写模式', module: 'word', item_key: '56:dictation', expected: '/words/review?word_id=56&mode=dictation' },
    { name: '单词例句直达词条例句', module: 'word', item_key: '56:example:34', expected: '/search/words/56#example-34' },
    { name: '课文报告回课文详情', module: 'lesson', item_key: '78', expected: '/lesson/78' },
    { name: '课文逐句报告指向原句', module: 'lesson', item_key: '78:sentence:3', expected: '/lesson/78#sentence-3' },
    { name: '课文第一句可定位', module: 'lesson', item_key: '78:sentence:0', expected: '/lesson/78#sentence-0' },
    { name: '影子跟读报告回影读页', module: 'shadowing', item_key: '78', expected: '/lesson/78/shadowing' },
    { name: '影子跟读逐句报告回同一篇影读页', module: 'shadowing', item_key: '78:v2:sentence:3', expected: '/lesson/78/shadowing' },
    { name: '写作报告按主题直达练习页', module: 'writing', item_key: 'topic-9:essay', expected: '/writing/practice?topic_id=topic-9' },
    { name: '写作主题含特殊字符时转义', module: 'writing', item_key: 'a b:sentence', expected: null },
    { name: '口语报告回口语练习页', module: 'speaking', item_key: '42', expected: '/speaking' },
    { name: '翻译报告回翻译练习页', module: 'translation', item_key: '42', expected: '/translation/practice' },
  ])('$name', ({ module, item_key, expected }) => {
    expect(errataSourcePath({ module, item_key })).toBe(expected)
  })
})

describe('errataSourcePath 拒绝无法安全定位的键', () => {
  it.each([
    { name: '未知模块', module: 'unknown', item_key: '12' },
    { name: '空定位键', module: 'grammar', item_key: '' },
    { name: '含路径穿越', module: 'grammar', item_key: '../admin' },
    { name: '含斜杠', module: 'lesson', item_key: '12/34' },
    { name: '单词主键非数字', module: 'word', item_key: 'abc:dictation' },
    { name: '语法主键非数字', module: 'grammar', item_key: 'abc' },
    { name: '超长定位键', module: 'grammar', item_key: '1'.repeat(81) },
    { name: '空分段', module: 'grammar', item_key: '12::34' },
  ])('$name 返回 null', ({ module, item_key }) => {
    expect(errataSourcePath({ module, item_key })).toBeNull()
  })

  it('所有可返回的链接都是站内相对路径', () => {
    const path = errataSourcePath({ module: 'grammar', item_key: '12:quiz:34' })
    expect(path?.startsWith('/')).toBe(true)
    expect(path?.startsWith('//')).toBe(false)
  })
})
