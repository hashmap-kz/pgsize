APP_NAME 	 := pgsize
OUTPUT   	 := $(APP_NAME)
INSTALL_DIR  := /usr/local/bin

ifeq ($(OS),Windows_NT)
	OUTPUT := $(APP_NAME).exe
endif

.PHONY: build
build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/$(OUTPUT) main.go

.PHONY: lint
lint:
	golangci-lint run --output.tab.path=stdout

.PHONY: install
install: build
	@echo "Installing bin/$(OUTPUT) to $(INSTALL_DIR)..."
	@sudo chmod +x bin/$(OUTPUT) && sudo cp bin/$(OUTPUT) $(INSTALL_DIR)

.PHONY: test
test:
	go test -v -race -cover -count=1 -timeout=5m ./...

.PHONY: test-integration
test-integration:
	go test -tags integration -v -count=1 -timeout=2m ./test/integration/...

.PHONY: clean
clean:
	@rm -rf bin/ dist/ *.log

.PHONY: snapshot
snapshot:
	GORELEASER_FORCE_TOKEN=github goreleaser release --skip sign --skip publish --snapshot --clean
