.PHONY: run test build lint seed front-build clean

run:        ## 启动开发服务器
	go run ./backend/cmd/server/

test:       ## 运行所有测试（含集成测试）
	go test ./... -v -count=1

build:      ## 编译服务端二进制
	go build -o bin/server ./backend/cmd/server/

lint:       ## 静态检查
	go vet ./...

seed:       ## 导入初始词库（N5/N4）
	go run ./internal/cli/ import-words --file ./data/seed/words_n5.json

front-build: ## 编译前端 TypeScript
	npx esbuild front/web/static/js/*.ts --bundle --outdir=front/web/static/js/dist

clean:
	rm -rf bin/ front/web/static/js/dist/
