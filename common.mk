PLATFORM 	:= $(shell go env | grep GOHOSTOS | cut -d '"' -f 2)
ARCH 		:= $(shell go env | grep GOARCH | cut -d '"' -f 2)
BRANCH		:= $(shell git rev-parse --abbrev-ref HEAD)
LDFLAGS 	:= -ldflags "-X main.Version $(VERSION) -X main.Name $(NAME)"

test:
	go test ./...

build:
	go build -o build/$(NAME) $(LDFLAGS) cmd/$(NAME).go

install:
	go install $(LDFLAGS)

compile:
	@rm -rf build/
	@gox $(LDFLAGS) \
	-os="darwin" \
	-os="linux" \
	-os="solaris" \
	-os="freebsd" \
	-output "build/{{.Dir}}_$(VERSION)_{{.OS}}_{{.Arch}}/$(NAME)" \
	./...

deps:
	go get github.com/c4milo/github-release
	go get github.com/mitchellh/gox
	go get github.com/BurntSushi/toml
	go get github.com/stretchr/graceful
	go get github.com/hashicorp/logutils
	go get github.com/c4milo/handlers/logger
	go get github.com/hooklift/assert
	go get gopkg.in/tylerb/graceful.v1

dist: compile
	$(eval FILES := $(shell ls build))
	@rm -rf dist && mkdir dist
	@for f in $(FILES); do \
		(cd $(shell pwd)/build/$$f && tar -cvzf ../../dist/$$f.tar.gz *); \
		(cd $(shell pwd)/dist && shasum -a 512 $$f.tar.gz > $$f.sha512); \
		echo $$f; \
	done

release: dist
	@latest_tag=$$(git describe --tags `git rev-list --tags --max-count=1`); \
	comparison="$$latest_tag..HEAD"; \
	if [ -z "$$latest_tag" ]; then comparison=""; fi; \
	changelog=$$(git log $$comparison --oneline --no-merges --reverse); \
	github-release c4milo/$(NAME) $(VERSION) $(BRANCH) "**Changelog**<br/>$$changelog" 'dist/*'; \
	git pull

.PHONY: test build install compile deps dist release
