PLATFORM 	:= $(shell go env | grep GOHOSTOS | cut -d '"' -f 2)
ARCH 		:= $(shell go env | grep GOARCH | cut -d '"' -f 2)
BRANCH		:= $(shell git rev-parse --abbrev-ref HEAD)
LDFLAGS 	:= -ldflags "-X main.Version $(VERSION) -X main.Name $(NAME)"

build:
	go build $(LDFLAGS) -o build/$(NAME)

install:
	go install $(LDFLAGS)

deps:
	go get github.com/c4milo/github-release

dist: build
	@rm -rf dist && mkdir dist
	@cp *.service build/
	(cd $(shell pwd)/build && tar -cvzf ../dist/$(NAME)_$(VERSION)_$(PLATFORM)_$(ARCH).tar.gz *); \
	(cd $(shell pwd)/dist && shasum -a 512 $(NAME)_$(VERSION)_$(PLATFORM)_$(ARCH).tar.gz > $(NAME)_$(VERSION)_$(PLATFORM)_$(ARCH).tar.gz.sha512);

release: dist
	@latest_tag=$$(git describe --tags `git rev-list --tags --max-count=1`); \
	comparison="$$latest_tag..HEAD"; \
	if [ -z "$$latest_tag" ]; then comparison=""; fi; \
	changelog=$$(git log $$comparison --oneline --no-merges --reverse); \
	github-release hooklift/hooklift $(VERSION) $(BRANCH) "**Changelog**<br/>$$changelog" 'dist/*'; \
	git pull

test:
	go test ./...

.PHONY: test build install deps dist release