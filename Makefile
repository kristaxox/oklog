GO_VERSION ?= $(shell go version | cut -c 12- | cut -d' ' -f1)
GOOS ?= linux
GOARCH ?= amd64
GOPATH ?= $(shell go env GOPATH)

GCP_PROJECT ?= storj-global

ifeq ($(VERSION),)
	BRANCH_NAME ?= $(shell git rev-parse --abbrev-ref HEAD | sed "s!/!-!g")
	ifeq (${BRANCH_NAME},main)
		TAG    := $(shell git rev-parse --short HEAD)-${GO_VERSION}
	else
		TAG    := $(shell git rev-parse --short HEAD)-${BRANCH_NAME}-${GO_VERSION}
	endif
else
	TAG    := $(shell git rev-parse --short HEAD)-${VERSION}-${GO_VERSION}
endif

.DEFAULT_GOAL := help
.PHONY: help
help:
	@awk 'BEGIN { \
		FS = ":.*##"; \
		printf "\nUsage:\n  make \033[36m<target>\033[0m\n"\
	} \
	/^[a-zA-Z_-]+:.*?##/ { \
		printf "  \033[36m%-17s\033[0m %s\n", $$1, $$2 \
	} \
	/^##@/ { \
		printf "\n\033[1m%s\033[0m\n", substr($$0, 5) \
	} ' $(MAKEFILE_LIST)


##@ Build

.PHONY: build
build: ## Build oklog binary
	@mkdir -p .build
	GOOS=${GOOS} GOARCH=${GOARCH} go build -o .build/oklog-${TAG}-${GOARCH} ./cmd/oklog
	
.PHONY: image
image: build ## Build oklog docker image
	echo Built version: ${TAG}
	docker build --build-arg TAG=${TAG} --build-arg GOARCH=${GOARCH} --pull=true -t gcr.io/${GCP_PROJECT}/oklog:${TAG}-${GOARCH} -f Dockerfile .	

### Test

.PHONY: test
test: ## run all tests
	go test ./...

##@ Deploy

.PHONY: push-image
push-image: image ## Push Docker images to storj-global GCR
	docker push gcr.io/${GCP_PROJECT}/oklog:${TAG}-amd64 
	@if [ ! -z "${VERSION}" ]; then \
		docker tag gcr.io/${GCP_PROJECT}/oklog:${TAG}-amd64 gcr.io/${GCP_PROJECT}/oklog:${VERSION} && \
		docker push gcr.io/${GCP_PROJECT}/oklog:${VERSION}; fi

##@ Clean

.PHONY: clean
clean: binaries-clean clean-images ## Clean local release binaries and local Docker images

.PHONY: binaries-clean
binaries-clean: ## Clean local binary
	@rm -rf .build

.PHONY: clean-images
clean-images: ## Clean local Docker images
	@docker rmi -f gcr.io/${GCP_PROJECT}/oklog:${TAG}-${GOARCH}
	@if [ ! -z "${VERSION}" ]; then \
		docker rmi -f gcr.io/${GCP_PROJECT}/oklog:${VERSION}; fi	
