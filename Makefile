.PHONY: default clean backend compute electron run flatpak flatpak-install

default: electron

run: electron
	./electron/dist/linux-unpacked/weave --no-sandbox

electron: backend compute
	@test -d electron/node_modules || (echo "Error: npm dependencies not installed" && echo "Run: cd electron && npm install" && exit 1)
	cd electron && npm run build

backend:
	go build -o build/weave-backend ./cmd/weave

compute:
	$(MAKE) -C compute

flatpak: electron
	flatpak-builder --force-clean build-dir packaging/flatpak/com.placeholder.weave.yml

flatpak-install: flatpak
	flatpak-builder --user --install --force-clean build-dir packaging/flatpak/com.placeholder.weave.yml

clean:
	rm -rf bin/
	rm -rf build/
	rm -rf electron/dist/
	rm -rf build-dir/
	$(MAKE) -C compute clean
