.PHONY: all
all: build

.PHONY: build
build:
	go build ./...

.PHONY: test
test:
	go test /...

.PHONY: test-ci
test-ci:
	go test /... -covermode=atomic -coverpkg=./... -coverprofile=./coverage.txt -json | tee output.txt

.PHONY: lint
lint:
	golangci-lint run $(addsuffix /...)

.PHONY: tidy
tidy:
	go mod tidy
