-include Local.mk

BINARY_NAME=routable-cni
PACKAGE=routable-cni
ORG_PATH=github.hpe.com/hpe
REPO_PATH=$(ORG_PATH)/$(PACKAGE)
GOPATH=$(CURDIR)/.gopath
GOBIN=$(CURDIR)/bin
BUILDDIR=$(CURDIR)/build
GOFILES = $(shell find . -name *.go | grep -vE "(\/vendor\/)|(_test.go)")
BASE=$(GOPATH)/src/$(REPO_PATH)

export GOPATH
export GOBIN
export GO111MODULE=on

IMAGEDIR=$(BASE)/images
DOCKERFILE=$(CURDIR)/images/Dockerfile
DEFAULT_TAG := bluedata/routable-cni:0.2
TAG ?= $(DEFAULT_TAG)

DOCKERARGS=
ifdef HTTP_PROXY
	DOCKERARGS += --build-arg http_proxy=$(HTTP_PROXY)
endif
ifdef HTTPS_PROXY
	DOCKERARGS += --build-arg https_proxy=$(HTTPS_PROXY)
endif

$(BASE): ; $(info  Setting GOPATH...)
	@mkdir -p $(dir $@)
	@ln -sf $(CURDIR) $@

$(GOBIN):
	@mkdir -p $@

$(BUILDDIR): | $(BASE) ; $(info Creating build directory...)
	@cd $(BASE) && mkdir -p $@

build: $(BUILDDIR)/$(BINARY_NAME) ; $(info Building $(BINARY_NAME)...) @
	$(info Done!)

$(BUILDDIR)/$(BINARY_NAME): $(GOFILES) | $(BUILDDIR)
	@cd $(BASE)/cmd/$(BINARY_NAME) && CGO_ENABLED=0 go build -o $(BUILDDIR)/$(BINARY_NAME) -tags no_openssl -v

modules:
	go mod tidy

tidy: modules

lint: | $(BASE) $(GOLINT) ; $(info  Running golint...) @
	$Q cd $(BASE) && ret=0 && for pkg in $(PKGS); do \
		test -z "$$($(GOLINT) $$pkg | tee /dev/stderr)" || ret=1 ; \
	 done ; exit $$ret

format: ; $(info  Running gofmt...) @
	@ret=0 && for d in $$(go list -f '{{.Dir}}' ./... | grep -v /vendor/); do \
		gofmt -l -w $$d/*.go || ret=$$? ; \
	 done ; exit $$ret

push:
	docker push ${TAG}

image: | $(BASE) ; $(info Building Docker image...) @
	@docker build -t $(TAG) -f $(DOCKERFILE)  $(CURDIR) $(DOCKERARGS)
	@sed -e 's~RELEASE_IMAGE_TAG~${TAG}~' images/routable-cni-ds-template.yaml >images/routable-cni-ds.yaml

clean: | $(BASE) ; $(info  Cleaning...) @
	@cd $(BASE) && go clean --modcache --cache --testcache
	@rm -rf $(GOPATH)
	@rm -rf $(BUILDDIR)
	@rm -rf $(GOBIN)
	@rm -f images/routable-cni-ds.yaml


.PHONY: build clean image push format clean lint modules tidy
