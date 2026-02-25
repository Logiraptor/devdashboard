.PHONY: build install

build:
	go build -o devdeploy ./cmd/devdeploy

install: build
	@echo "Installing binaries to ~/bin/"
	@mkdir -p ~/bin
	@cp devdeploy ~/bin/
	@echo "Make sure ~/bin is in your PATH"
