.PHONY: build run clean

build:
	cd web && npm run build
	go build -o instant-share .

run: build
	./instant-share

clean:
	rm -f instant-share
	rm -rf web/dist