.PHONY: build install clean

build:
	go build -o build/TunProxy.app/Contents/MacOS/tun-proxy .

install: build
	cp -r build/TunProxy.app /Applications/TunProxy.app
	cp $(HOME)/sing-box-test/sing-box $(HOME)/.tun-proxy/sing-box 2>/dev/null || true

clean:
	rm -rf build/TunProxy.app/Contents/MacOS/tun-proxy
