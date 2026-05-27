.PHONY: build test vet lint install run-example

build:
	go build -o koto ./cmd/koto

test:
	go test -race -cover ./...

vet:
	go vet ./...

install:
	go install ./cmd/koto

clean:
	rm -f koto
