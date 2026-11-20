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
	# La config de swayimg se copia: es del usuario, no del proyecto.
	mkdir -p $(PREFIX)/.config/swayimg/videos
	cp -r swayimg-config-video/init.lua swayimg-config-video/lib \
		$(PREFIX)/.config/swayimg/videos/

uninstall:
	rm -f $(PREFIX)/bin/videocli
	rm -rf $(PREFIX)/share/videocli

clean:
	rm -f $(BIN)
