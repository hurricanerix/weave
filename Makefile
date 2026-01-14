.PHONY: default clean build weave compute

default: clean build

build: weave compute

weave:
	go build -o bin/weave ./cmd/weave

compute:
	$(MAKE) -C compute-daemon

clean:
	rm -f bin/weave
	$(MAKE) -C compute-daemon clean
