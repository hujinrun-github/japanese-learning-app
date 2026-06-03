.PHONY: run test build lint seed seed-grammar seed-lessons seed-speaking seed-writing seed-all front-build clean kill-ports start-all start-backend start-admin start-learner-front start-admin-front

# ---- 端口配置 ----
MAIN_PORT    := 8081
ADMIN_PORT   := 8082
LEARNER_PORT := 5173
ADMIN_FRONT_PORT := 5174

ADMIN_TOKEN  ?= change-me
LOG_DIR      := logs

# ---- 开发命令 ----
run:        ## 启动开发服务器
	go run ./backend/cmd/server/

test:       ## 运行所有测试（含集成测试）
	go test ./... -v -count=1

build:      ## 编译服务端二进制
	go build -o bin/server ./backend/cmd/server/

lint:       ## 静态检查
	go vet ./...

# ---- 种子数据 ----
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

# ---- 前端构建 ----
front-build: ## 编译前端 TypeScript
	npx esbuild front/web/static/js/*.ts --bundle --outdir=front/web/static/js/dist

# ---- 管理后台 ----
admin-run:     ## 启动管理后台
	ADMIN_TOKEN=$(ADMIN_TOKEN) go run ./backend/cmd/admin/

admin-build:   ## 编译管理后台
	go build -o bin/admin ./backend/cmd/admin/

admin-front-build: ## 构建管理后台前端
	cd front/admin && npm install && npm run build

admin-front-dev: ## 启动管理后台前端开发服务器
	cd front/admin && npm run dev

# ---- 一键启停 ----
kill-ports: ## 杀死所有相关端口的进程
	@echo "==> Killing old processes..."
	@for port in $(MAIN_PORT) $(ADMIN_PORT) $(LEARNER_PORT) $(ADMIN_FRONT_PORT); do \
		pid=$$(lsof -ti :$$port 2>/dev/null); \
		if [ -n "$$pid" ]; then \
			kill -9 $$pid 2>/dev/null && echo "  killed port $$port (pid $$pid)"; \
		else \
			echo "  port $$port — already free"; \
		fi; \
	done

start-backend: ## 启动主后端 (8081)
	@mkdir -p $(LOG_DIR)
	@echo "==> Starting main server on :$(MAIN_PORT)..."
	@nohup go run ./backend/cmd/server/ > $(LOG_DIR)/server.log 2>&1 &
	@sleep 1
	@if lsof -ti :$(MAIN_PORT) >/dev/null 2>&1; then \
		echo "  main server started ✓ (log: $(LOG_DIR)/server.log)"; \
	else \
		echo "  main server may have failed, check $(LOG_DIR)/server.log"; \
	fi

start-admin: ## 启动管理后台 (8082)
	@mkdir -p $(LOG_DIR)
	@echo "==> Starting admin server on :$(ADMIN_PORT)..."
	@ADMIN_TOKEN=$(ADMIN_TOKEN) nohup go run ./backend/cmd/admin/ > $(LOG_DIR)/admin.log 2>&1 &
	@sleep 1
	@if lsof -ti :$(ADMIN_PORT) >/dev/null 2>&1; then \
		echo "  admin server started ✓ (log: $(LOG_DIR)/admin.log)"; \
	else \
		echo "  admin server may have failed, check $(LOG_DIR)/admin.log"; \
	fi

start-learner-front: ## 启动学习者前端 (5173)
	@mkdir -p $(LOG_DIR)
	@echo "==> Starting learner frontend on :$(LEARNER_PORT)..."
	@cd front/react && nohup npm run dev > ../../$(LOG_DIR)/learner-front.log 2>&1 &
	@sleep 2
	@if lsof -ti :$(LEARNER_PORT) >/dev/null 2>&1; then \
		echo "  learner frontend started ✓ (log: $(LOG_DIR)/learner-front.log)"; \
	else \
		echo "  learner frontend may have failed, check $(LOG_DIR)/learner-front.log"; \
	fi

start-admin-front: ## 启动管理平台前端 (5174)
	@mkdir -p $(LOG_DIR)
	@echo "==> Starting admin frontend on :$(ADMIN_FRONT_PORT)..."
	@cd front/admin && nohup npm run dev > ../../$(LOG_DIR)/admin-front.log 2>&1 &
	@sleep 2
	@if lsof -ti :$(ADMIN_FRONT_PORT) >/dev/null 2>&1; then \
		echo "  admin frontend started ✓ (log: $(LOG_DIR)/admin-front.log)"; \
	else \
		echo "  admin frontend may have failed, check $(LOG_DIR)/admin-front.log"; \
	fi

start-all: kill-ports start-backend start-admin start-learner-front start-admin-front ## 一键启动全部服务
	@echo ""
	@echo "============================================"
	@echo "  All services started!"
	@echo "  Learner:   http://localhost:$(LEARNER_PORT)"
	@echo "  Admin UI:  http://localhost:$(ADMIN_FRONT_PORT)"
	@echo "  Main API:  http://localhost:$(MAIN_PORT)"
	@echo "  Admin API: http://localhost:$(ADMIN_PORT)"
	@echo "  Logs:      $(LOG_DIR)/"
	@echo "============================================"

# ---- 清理 ----
clean:
	rm -rf bin/ front/web/static/js/dist/
