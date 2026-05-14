.PHONY: build build-helper install clean

build: build-helper
	go build -o build/TunProxy.app/Contents/MacOS/tun-proxy .
	cp helper/tun-proxy-helper build/TunProxy.app/Contents/Resources/
	cp helper/com.hanger.tun-proxy.helper.plist build/TunProxy.app/Contents/Resources/

build-helper:
	cd helper && go build -o tun-proxy-helper .

install: build
	cp -r build/TunProxy.app /Applications/TunProxy.app

clean:
	rm -rf build/TunProxy.app/Contents/MacOS/tun-proxy
	rm -f helper/tun-proxy-helper
