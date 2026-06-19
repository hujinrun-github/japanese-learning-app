.PHONY: run web test build lint seed seed-grammar seed-lessons seed-speaking seed-writing seed-all seed-postgres postgres-migrate storage-reconcile validate-shadowing-pilot front-build clean kill-ports start-all start-backend start-admin start-learner-front start-admin-front admin-run admin-build admin-front-build admin-front-dev

MAIN_PORT := 30081
ADMIN_PORT := 30082
LEARNER_PORT := 35173
ADMIN_FRONT_PORT := 35174

ADMIN_TOKEN ?= change-me
LOG_DIR := logs

run: ## Start the backend server
	RELATIONAL_STORE=$(RELATIONAL_STORE) DATABASE_URL=$(DATABASE_URL) LISTEN_ADDR=:$(MAIN_PORT) APP_BASE_URL=http://localhost:$(LEARNER_PORT) go run ./backend/cmd/server/

web: build ## Build the web service binary

test: ## Run all Go tests
	go test ./... -v -count=1

build: ## Build the backend binary
	go build -o bin/server ./backend/cmd/server/

lint: ## Run go vet
	go vet ./...

seed: ## Import seed words
	go run ./backend/cmd/appctl import-words --file ./data/seed/words_n5.json

seed-grammar: ## Import seed grammar
	go run ./backend/cmd/appctl import-grammar --file ./data/seed/grammar_n5.json
	go run ./backend/cmd/appctl import-grammar --file ./data/seed/grammar_n4.json
	go run ./backend/cmd/appctl import-grammar --file ./data/seed/grammar_n3.json

seed-lessons: ## Import seed lessons
	go run ./backend/cmd/appctl import-lessons --file ./data/seed/lessons_n5.json
	go run ./backend/cmd/appctl import-lessons --file ./data/seed/lessons_n4.json
	go run ./backend/cmd/appctl import-lessons --file ./data/seed/lessons_n3.json

seed-speaking: ## Import seed speaking materials
	go run ./backend/cmd/appctl import-speaking --file ./data/seed/speaking_materials.json

seed-writing: ## Import seed writing questions
	go run ./backend/cmd/appctl import-writing --file ./data/seed/writing_questions.json

seed-all: seed seed-grammar seed-lessons seed-speaking seed-writing ## Import all seed data

seed-postgres: ## Import PostgreSQL seed data
	DATABASE_URL=$(DATABASE_URL) go run ./backend/cmd/appctl import-lessons-postgres --file ./data/seed/lessons_shadowing_pilot.json

postgres-migrate: ## Show SQLite to relational migration help
	go run ./backend/cmd/storage-migrate migrate-sqlite-to-relational --help

storage-reconcile: ## Dry-run audio object reconciliation
	go run ./backend/cmd/storage-migrate audio-reconcile --dry-run

validate-shadowing-pilot: ## Validate the shadowing pilot lesson material pack
	python scripts/validate_lessons_shadowing.py --file ./data/seed/lessons_shadowing_pilot.json

front-build: ## Build legacy frontend TypeScript assets
	npx esbuild front/web/static/js/*.ts --bundle --outdir=front/web/static/js/dist

admin-run: ## Start the admin API
	ADMIN_TOKEN=$(ADMIN_TOKEN) LISTEN_ADDR=:$(ADMIN_PORT) go run ./backend/cmd/admin/

admin-build: ## Build the admin API binary
	go build -o bin/admin ./backend/cmd/admin/

admin-front-build: ## Build the admin frontend
	cd front/admin && npm install && npm run build

admin-front-dev: ## Start the admin frontend dev server
	cd front/admin && npm run dev

kill-ports: ## Kill processes bound to the app ports
	@echo "==> Killing old processes..."
	@for port in $(MAIN_PORT) $(ADMIN_PORT) $(LEARNER_PORT) $(ADMIN_FRONT_PORT); do \
		pid=$$(lsof -ti :$$port 2>/dev/null); \
		if [ -n "$$pid" ]; then \
			kill -9 $$pid 2>/dev/null && echo "  killed port $$port (pid $$pid)"; \
		else \
			echo "  port $$port already free"; \
		fi; \
	done

start-backend: ## Start the main backend on :30081
	@mkdir -p $(LOG_DIR)
	@echo "==> Starting main server on :$(MAIN_PORT)..."
	@RELATIONAL_STORE=$(RELATIONAL_STORE) DATABASE_URL=$(DATABASE_URL) LISTEN_ADDR=:$(MAIN_PORT) APP_BASE_URL=http://localhost:$(LEARNER_PORT) nohup go run ./backend/cmd/server/ > $(LOG_DIR)/server.log 2>&1 &
	@sleep 1
	@if lsof -ti :$(MAIN_PORT) >/dev/null 2>&1; then \
		echo "  main server started (log: $(LOG_DIR)/server.log)"; \
	else \
		echo "  main server may have failed, check $(LOG_DIR)/server.log"; \
	fi

start-admin: ## Start the admin API on :30082
	@mkdir -p $(LOG_DIR)
	@echo "==> Starting admin server on :$(ADMIN_PORT)..."
	@ADMIN_TOKEN=$(ADMIN_TOKEN) LISTEN_ADDR=:$(ADMIN_PORT) nohup go run ./backend/cmd/admin/ > $(LOG_DIR)/admin.log 2>&1 &
	@sleep 1
	@if lsof -ti :$(ADMIN_PORT) >/dev/null 2>&1; then \
		echo "  admin server started (log: $(LOG_DIR)/admin.log)"; \
	else \
		echo "  admin server may have failed, check $(LOG_DIR)/admin.log"; \
	fi

start-learner-front: ## Start the learner frontend on :35173
	@mkdir -p $(LOG_DIR)
	@echo "==> Starting learner frontend on :$(LEARNER_PORT)..."
	@cd front/react && nohup npm run dev > ../../$(LOG_DIR)/learner-front.log 2>&1 &
	@sleep 2
	@if lsof -ti :$(LEARNER_PORT) >/dev/null 2>&1; then \
		echo "  learner frontend started (log: $(LOG_DIR)/learner-front.log)"; \
	else \
		echo "  learner frontend may have failed, check $(LOG_DIR)/learner-front.log"; \
	fi

start-admin-front: ## Start the admin frontend on :35174
	@mkdir -p $(LOG_DIR)
	@echo "==> Starting admin frontend on :$(ADMIN_FRONT_PORT)..."
	@cd front/admin && nohup npm run dev > ../../$(LOG_DIR)/admin-front.log 2>&1 &
	@sleep 2
	@if lsof -ti :$(ADMIN_FRONT_PORT) >/dev/null 2>&1; then \
		echo "  admin frontend started (log: $(LOG_DIR)/admin-front.log)"; \
	else \
		echo "  admin frontend may have failed, check $(LOG_DIR)/admin-front.log"; \
	fi

start-all: kill-ports start-backend start-admin start-learner-front start-admin-front ## Start all local services
	@echo ""
	@echo "============================================"
	@echo "  All services started!"
	@echo "  Learner:   http://localhost:$(LEARNER_PORT)"
	@echo "  Admin UI:  http://localhost:$(ADMIN_FRONT_PORT)"
	@echo "  Main API:  http://localhost:$(MAIN_PORT)"
	@echo "  Admin API: http://localhost:$(ADMIN_PORT)"
	@echo "  Logs:      $(LOG_DIR)/"
	@echo "============================================"

clean: ## Remove build artifacts
	rm -rf bin/ front/web/static/js/dist/
