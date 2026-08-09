.DEFAULT_GOAL := build
bin=loxicmd
dock?=loxilb
SHELL := /bin/bash

# ── Version ──────────────────────────────────────────────────────────────────
# loxicmd-inference-gateway versions in lockstep with loxilb-inference-gateway
# and uses the same vMAJOR.MINOR.PATCH[.BUILD] scheme: CLI vX ships against
# gateway vX. This is what a local `make build` stamps into the binary
# (cmd.Version); a release build takes the version from the git tag instead
# (.github/workflows/release.yml).
#   make build VERSION=v0.9.8.7
VERSION ?= v0.9.8.6
BUILDINFO = $(shell date '+%Y_%m_%d')-$(shell git branch --show-current)-$(shell git show --pretty=format:%h --no-patch)
LDFLAGS   = -X 'github.com/loxilb-io/loxicmd-inference-gateway/cmd.Version=$(VERSION)' \
            -X 'github.com/loxilb-io/loxicmd-inference-gateway/cmd.BuildInfo=$(BUILDINFO)'

loxilbid=$(shell docker ps -f name=$(dock) | grep -w $(dock) | cut  -d " "  -f 1 | grep -iv  "CONTAINER")

.PHONY: build test check lint run install clean version docker-cp

build:
	@go build -o ${bin} -ldflags="$(LDFLAGS)"

test:
	go test ./...

check:
	go test ./...

lint:
	golangci-lint run --timeout=5m ./...

run:
	./$(bin)

version:
	@echo $(VERSION)

install:
	cp loxicmd /usr/local/sbin/
	/usr/local/sbin/loxicmd completion bash > /etc/bash_completion.d/loxicmd
	source /etc/bash_completion.d/loxicmd

clean:
	rm -f $(bin)

docker-cp: build
	docker cp loxicmd $(loxilbid):/usr/local/sbin/loxicmd
