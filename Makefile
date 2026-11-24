MODULE_DIR := videocli
BIN := $(MODULE_DIR)/videocli
PREFIX ?= $(HOME)/.local

.PHONY: build test vet check install uninstall clean

build:
	CGO_ENABLED=0 go build -C $(MODULE_DIR) -o videocli ./cmd/videocli

test:
	go test -C $(MODULE_DIR) ./...

vet:
	go vet -C $(MODULE_DIR) ./...

check: vet test build

# Instala solo el binario; la config de swayimg se lanza directamente con
# swayimg --config ./swayimg-config-video/init.lua desde este directorio.
install: build
	ln -sf $(abspath $(BIN)) $(PREFIX)/bin/videocli
	# La config de swayimg vive en ~/.config/swayimg/videos (se gestiona ahí,
	# no se copia desde el repo).

uninstall:
	rm -f $(PREFIX)/bin/videocli
	rm -rf $(PREFIX)/share/videocli

clean:
	rm -f $(BIN)
