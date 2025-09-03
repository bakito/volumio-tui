# Include toolbox tasks
include ./.toolbox.mk

# Run go golanci-lint
lint: tb.golangci-lint
	$(TB_GOLANGCI_LINT) run --fix


build:
	go build -o volumio-tui

release: tb.goreleaser tb.semver
	@version=$$($(TB_SEMVER)); \
	git tag -s $$version -m"Release $$version"
	$(TB_GORELEASER) --clean

test-release: tb.goreleaser
	$(TB_GORELEASER) --skip=publish --snapshot --clean
