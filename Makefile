# dsh-tui — deepseek-harness 终端客户端
#
# 环境变量: GOMODCACHE/GOPATH/GOCACHE 指向工作区内目录(沙箱/离线友好)。
# 仓库 .git 目录状态特殊,构建需 -buildvcs=false。

GO ?= go
export GOMODCACHE := $(CURDIR)/.modcache
export GOPATH := $(CURDIR)/.gopath
export GOCACHE := $(CURDIR)/.gocache
GOFLAGS := -buildvcs=false

.PHONY: build test vet run dump spike clean

build:
	$(GO) build $(GOFLAGS) -o bin/dsh-tui ./cmd/dsh-tui

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
