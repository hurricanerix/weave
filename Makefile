.PHONY: default clean backend compute electron run flatpak flatpak-install l5-relay

default: electron

run: electron
	WEAVE_CONFIG_DIR=./config ./electron/dist/linux-unpacked/weave --no-sandbox

electron: backend compute
	@test -d electron/node_modules || (echo "Error: npm dependencies not installed" && echo "Run: cd electron && npm install" && exit 1)
	cd electron && npm run build

backend:
	mkdir -p backend/bin
	cd backend && go build -o bin/weave-backend ./cmd/weave

compute:
	$(MAKE) -C compute

l5-relay:
	mkdir -p l5/bin
	cd l5 && go build -o bin/l5-relay ./cmd/l5-relay

flatpak: electron
	flatpak-builder --force-clean build-dir packaging/flatpak/com.placeholder.weave.yml

flatpak-install: flatpak
	flatpak-builder --user --install --force-clean build-dir packaging/flatpak/com.placeholder.weave.yml

clean:
	rm -rf backend/bin/
	rm -rf l5/bin/
	rm -rf electron/dist/
	rm -rf build-dir/
	$(MAKE) -C compute clean
