BIN := bin/gocomments

.PHONY: build test vet fmt clean

build:
	CGO_ENABLED=0 go build -o $(BIN) ./cmd/gocomments

test:
	go test ./... -count=1

vet:
	go vet ./...

fmt:
	gofmt -l -w .

clean:
	rm -rf bin
