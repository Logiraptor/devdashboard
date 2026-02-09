.PHONY: build ralph install

build:
	go build -o devdeploy ./cmd/devdeploy

ralph:
	go build -o ralph ./cmd/ralph

install: build ralph
	@echo "Installing binaries to ~/bin/"
	@mkdir -p ~/bin
	@cp devdeploy ~/bin/
	@cp ralph ~/bin/
	@echo "Make sure ~/bin is in your PATH"
