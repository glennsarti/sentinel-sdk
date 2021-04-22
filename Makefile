GOTOOLS = \
	github.com/golang/protobuf/protoc-gen-go@latest \
	github.com/vektra/mockery/cmd/mockery@latest \
	gotest.tools/gotestsum@latest

SENTINEL_VERSION = 0.18.0
SENTINEL_BIN_PATH := $(shell go env GOPATH)/bin

test: tools
	gotestsum --format=short-verbose $(TEST) $(TESTARGS)

generate: tools
	go generate ./...

modules:
	go mod download && go mod verify

test-circle:
	mkdir -p test-results/sentinel-sdk
	gotestsum --format=short-verbose --junitfile test-results/sentinel-sdk/results.xml

tools:
	@echo $(GOTOOLS) | xargs -t -n1 go install
	go mod tidy

$(SENTINEL_BIN_PATH)/sentinel:
	gpg --import .circleci/hashicorp.gpg && \
	cd /tmp && \
	curl -O https://releases.hashicorp.com/sentinel/${SENTINEL_VERSION}/sentinel_${SENTINEL_VERSION}_linux_amd64.zip && \
	curl -O https://releases.hashicorp.com/sentinel/${SENTINEL_VERSION}/sentinel_${SENTINEL_VERSION}_SHA256SUMS && \
	curl -O https://releases.hashicorp.com/sentinel/${SENTINEL_VERSION}/sentinel_${SENTINEL_VERSION}_SHA256SUMS.sig && \
	gpg --verify sentinel_${SENTINEL_VERSION}_SHA256SUMS.sig && \
	shasum --check --ignore-missing sentinel_${SENTINEL_VERSION}_SHA256SUMS && \
	cd $(SENTINEL_BIN_PATH) && \
	unzip /tmp/sentinel_${SENTINEL_VERSION}_linux_amd64.zip && \
	cd && which sentinel

.PHONY: test generate modules test-circle tools
