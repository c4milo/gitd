GHACCOUNT := c4milo
NAME := gitd
VERSION := v2.0.0

include common.mk

deps:
	go get github.com/c4milo/github-release
	go get github.com/mitchellh/gox
	go get github.com/BurntSushi/toml
	go get github.com/hashicorp/logutils
	go get github.com/c4milo/handlers/logger
	go get github.com/hooklift/assert
	go get gopkg.in/tylerb/graceful.v1
