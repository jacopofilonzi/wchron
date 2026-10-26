# Entry point for every command (`make help`). Build logic lives in Taskfile.yml
# (run through `wails3 task`); this file only wraps it.
#
# Recipes must work in both sh and cmd.exe (GNU make on Windows uses cmd.exe
# when no sh.exe is in the PATH): only `cd dir && cmd`, no inline `VAR=x cmd`,
# no rm/cp/mkdir -p (file operations go through `node -e`).

# Dev overrides (.env is git-ignored, see .env.example).
-include .env
export

GOPATH := $(shell go env GOPATH)
HOSTARCH := $(shell go env GOARCH)

# A 32-bit Go cannot run Wails: build and test as amd64.
ifeq ($(HOSTARCH),386)
  ARCH ?= amd64
else
  ARCH ?= $(HOSTARCH)
endif
GOARCH := $(ARCH)

# The Wails tasks call wails3 by name: make sure GOPATH/bin is in the PATH.
ifeq ($(OS),Windows_NT)
  PATH := $(GOPATH)\bin;$(PATH)
  MAKENSIS ?= C:/Program Files (x86)/NSIS/makensis.exe
else
  PATH := $(GOPATH)/bin:$(PATH)
  MAKENSIS ?= makensis
endif

# Packages without Wails, vetted for Linux too (the platform stubs must compile).
CROSS_PKGS = ./locales/... ./internal/apperr/... ./internal/i18n/... ./internal/platform/... ./internal/logging/... \
  ./internal/version/... ./internal/devenv/... ./internal/fsutil/... ./internal/migrate/... ./internal/config/... \
  ./internal/state/... ./internal/backup/... ./internal/startup/... ./internal/clock/... ./internal/tokens/...

# go:embed needs frontend/dist to hold a file before the frontend is built.
DIST_GUARD = node -e "const fs=require('fs');fs.mkdirSync('frontend/dist',{recursive:true});if(!fs.readdirSync('frontend/dist').length)fs.writeFileSync('frontend/dist/.keep','')"

.DEFAULT_GOAL := help
.PHONY: help deps dev build package run bindings test vet vet-cross check version clean

help: ## Show this list
	@node -e "for (const l of require('fs').readFileSync('Makefile','utf8').split(/\r?\n/)) { const m = /^([a-z-]+):.*## (.*)$$/.exec(l); if (m) console.log('  ' + m[1].padEnd(10) + ' ' + m[2]); }"

deps: ## Install the frontend dependencies
	cd frontend && npm install

dev: ## Run the app with hot reload (wails3 dev)
	wails3 task dev

build: ## Build bin/wchron.exe
	wails3 task build ARCH=$(ARCH)

package: ## Build bin/wchron.exe and the per-user installer bin/wchron-amd64-installer.exe
	wails3 task package ARCH=$(ARCH) "MAKENSIS=$(MAKENSIS)"

run: build ## Build and start the app
	wails3 task run

bindings: ## Regenerate frontend/bindings from the Go service
	$(DIST_GUARD)
	wails3 generate bindings -clean=true -ts -i

test: ## Go tests
	$(DIST_GUARD)
	go test ./...

vet: ## go vet for Windows, and for Linux on the packages without Wails
	$(DIST_GUARD)
	go vet ./...
	$(MAKE) --no-print-directory vet-cross

vet-cross: export GOOS := linux
vet-cross:
	go vet $(CROSS_PKGS)

check: bindings vet test ## Everything that must pass: vet, tests, svelte-check, frontend build
	cd frontend && npm run check
	cd frontend && npm run build

version: ## Set the version everywhere: make version V=x.y.z
	node scripts/set-version.mjs $(V)

clean: ## Remove the build output
	node -e "const fs=require('fs');for(const d of ['bin','frontend/dist','.task'])fs.rmSync(d,{recursive:true,force:true})"
