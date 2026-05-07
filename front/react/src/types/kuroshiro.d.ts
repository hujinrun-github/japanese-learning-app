declare module 'kuroshiro' {
  interface ConvertOptions {
    mode?: 'normal' | 'spaced' | 'okurigana' | 'furigana'
    to?: 'hiragana' | 'katakana' | 'romaji'
    delimiter_start?: string
    delimiter_end?: string
  }

  export default class Kuroshiro {
    init(analyzer: any): Promise<void>
    convert(text: string, options?: ConvertOptions): Promise<string>
  }
}

declare module 'kuroshiro-analyzer-kuromoji' {
  export default class KuromojiAnalyzer {
    constructor(options?: { dictPath?: string })
  }
}
