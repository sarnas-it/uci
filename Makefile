# uci — кроссплатформенный загрузчик «одна ссылка → файл».
# Сборка статических бинарей под три ОС: make build (все), make build-linux и т.п.

.PHONY: build build-linux build-macos build-windows test vet clean

BIN_DIR := bin

build: build-linux build-macos build-windows

build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o $(BIN_DIR)/uci-linux-amd64 .
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -o $(BIN_DIR)/uci-linux-arm64 .

build-macos:
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o $(BIN_DIR)/uci-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -o $(BIN_DIR)/uci-darwin-arm64 .

build-windows:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o $(BIN_DIR)/uci-windows-amd64.exe .

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -rf $(BIN_DIR)
