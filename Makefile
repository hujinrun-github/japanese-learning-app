.PHONY: run test build lint seed seed-grammar seed-lessons seed-speaking seed-writing seed-all front-build clean

run:        ## 启动开发服务器
	go run ./backend/cmd/server/

test:       ## 运行所有测试（含集成测试）
	go test ./... -v -count=1

build:      ## 编译服务端二进制
	go build -o bin/server ./backend/cmd/server/

lint:       ## 静态检查
	go vet ./...

seed:       ## 导入初始词库（N5/N4）
	go run ./backend/cmd/server/ import-words --file ./data/seed/words_n5.json

seed-grammar: ## 导入语法点（N5/N4/N3）
	go run ./backend/cmd/server/ import-grammar --file ./data/seed/grammar_n5.json
	go run ./backend/cmd/server/ import-grammar --file ./data/seed/grammar_n4.json
	go run ./backend/cmd/server/ import-grammar --file ./data/seed/grammar_n3.json

seed-lessons: ## 导入阅读课文（N5/N4/N3）
	go run ./backend/cmd/server/ import-lessons --file ./data/seed/lessons_n5.json
	go run ./backend/cmd/server/ import-lessons --file ./data/seed/lessons_n4.json
	go run ./backend/cmd/server/ import-lessons --file ./data/seed/lessons_n3.json

seed-speaking: ## 导入口语材料
	go run ./backend/cmd/server/ import-speaking --file ./data/seed/speaking_materials.json

seed-writing: ## 导入写作题
	go run ./backend/cmd/server/ import-writing --file ./data/seed/writing_questions.json

seed-all: seed seed-grammar seed-lessons seed-speaking seed-writing ## 导入所有种子数据

front-build: ## 编译前端 TypeScript
	npx esbuild front/web/static/js/*.ts --bundle --outdir=front/web/static/js/dist

clean:
	rm -rf bin/ front/web/static/js/dist/
