# dsh-tui — deepseek-harness 终端客户端
#
# 环境变量: GOMODCACHE/GOPATH/GOCACHE 指向工作区内目录(沙箱/离线友好)。
# 仓库 .git 目录状态特殊,构建需 -buildvcs=false。

GO ?= go
VERSION ?= $(shell git describe --tags --dirty --always 2>/dev/null || echo dev)
export GOMODCACHE := $(CURDIR)/.modcache
export GOPATH := $(CURDIR)/.gopath
export GOCACHE := $(CURDIR)/.gocache
GOFLAGS := -buildvcs=false
LDFLAGS := -s -w -X github.com/Menfre01/dsh-tui/internal/tui.Version=$(VERSION)

.PHONY: build test vet run dump spike clean

build:
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/dsh-tui ./cmd/dsh-tui

test:
	$(GO) test $(GOFLAGS) ./...

vet:
	$(GO) vet $(GOFLAGS) ./...

run: build
	./bin/dsh-tui

dump: build
	./bin/dsh-tui --dump

spike:
	$(GO) build $(GOFLAGS) -o bin/dsh-spike ./cmd/dsh-spike

clean:
	rm -rf bin

# ---------------------------------------------------------------------------
# 发布:交叉编译 3 平台 × 2 架构,打包 tar.gz/zip + checksums.txt
# 产物在 dist/(配合 install.sh / install.ps1 使用)
# ---------------------------------------------------------------------------

GOOSES   = linux darwin windows
GOARCHES = amd64 arm64
BINARY   = dsh-tui
DIST_DIR = dist

.PHONY: release
release:
	@rm -rf $(DIST_DIR)
	@mkdir -p $(DIST_DIR)
	@for GOOS in $(GOOSES); do \
		for GOARCH in $(GOARCHES); do \
			echo "→ Building $$GOOS/$$GOARCH ..."; \
			GOOS=$$GOOS GOARCH=$$GOARCH CGO_ENABLED=0 \
				$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY) ./cmd/dsh-tui; \
			if [ "$$GOOS" = "windows" ]; then \
				mv $(DIST_DIR)/$(BINARY) $(DIST_DIR)/$(BINARY).exe; \
				cd $(DIST_DIR) && zip $(BINARY)_$${GOOS}_$${GOARCH}.zip $(BINARY).exe && rm $(BINARY).exe; \
				cd $(CURDIR); \
			else \
				tar -czf $(DIST_DIR)/$(BINARY)_$${GOOS}_$${GOARCH}.tar.gz \
					-C $(DIST_DIR) $(BINARY); \
				rm $(DIST_DIR)/$(BINARY); \
			fi; \
		done; \
	done
	@cd $(DIST_DIR) && shasum -a 256 *.tar.gz *.zip > checksums.txt
	@echo "Done → $(DIST_DIR)/"

.PHONY: homebrew-formula
homebrew-formula:
	@chmod +x .github/scripts/generate-formula.sh
	@.github/scripts/generate-formula.sh > /tmp/dsh-tui.rb
	@echo "Formula → /tmp/dsh-tui.rb"
