.PHONY: dev build clean tidy bindings install

dev:
	wails dev

build:
	wails build

clean:
	rm -rf build/bin frontend/dist

tidy:
	go mod tidy
	cd frontend && npm install

bindings:
	wails generate module

install:
	cd frontend && npm install
