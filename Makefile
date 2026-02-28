GO ?= go
PKGS := ./...
BIN_DIR ?= bin
CLOCKCTL_BIN ?= $(BIN_DIR)/clockctl
IMAGE_NAME ?= clock-server
IMAGE_TAG ?= 0.0.1
IMAGE ?= $(IMAGE_NAME):$(IMAGE_TAG)
GO_IMAGE ?= golang:1.24.2-alpine
RUNTIME_IMAGE ?= alpine:3.21
DOCKER_BUILD_ARGS ?= --build-arg GO_IMAGE=$(GO_IMAGE) --build-arg RUNTIME_IMAGE=$(RUNTIME_IMAGE)
REGISTRY ?= 192.168.2.201:32000
REGISTRY_REPO ?= $(IMAGE_NAME)
REGISTRY_IMAGE ?= $(REGISTRY)/$(REGISTRY_REPO):$(IMAGE_TAG)
LATEST_TAG ?= latest
REGISTRY_IMAGE_LATEST ?= $(REGISTRY)/$(REGISTRY_REPO):$(LATEST_TAG)
CONTAINER_NAME ?= clock-server
HOST_PORT ?= 8080
CONTAINER_PORT ?= 8080
HELM ?= helm
HELM_RELEASE ?= clock-server
HELM_NAMESPACE ?= clock-system
HELM_CHART ?= ./helm/clock-server
HELM_VALUES ?=

.PHONY: all
all: fmt mod test build

.PHONY: fmt
fmt:
	$(GO) fmt $(PKGS)

.PHONY: mod
mod:
	$(GO) mod tidy

.PHONY: test unit-test cucumber-test
test: unit-test cucumber-test

unit-test:
	$(GO) test $(PKGS)

cucumber-test:
	$(GO) test ./internal/api -run TestFeatures -v

.PHONY: build
build:
	$(GO) build $(PKGS)

.PHONY: build-clockctl
build-clockctl:
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(CLOCKCTL_BIN) ./cmd/clockctl

.PHONY: clean
clean:
	rm -rf bin dist build

.PHONY: docker-build
docker-build:
	docker build $(DOCKER_BUILD_ARGS) -t $(IMAGE) .

.PHONY: docker-login
docker-login:
	@if [ -n "$(REGISTRY)" ]; then \
		echo "Logging in to $(REGISTRY)"; \
		docker login $(REGISTRY); \
	else \
		echo "Registry not set. Please enter your registry details."; \
		printf "Registry (e.g. ghcr.io): "; read registry; \
		docker login $$registry; \
	fi

.PHONY: docker-push
docker-push: docker-build
	docker tag $(IMAGE) $(REGISTRY_IMAGE)
	docker tag $(IMAGE) $(REGISTRY_IMAGE_LATEST)
	docker push $(REGISTRY_IMAGE)
	docker push $(REGISTRY_IMAGE_LATEST)

.PHONY: docker-run
docker-run: docker-build
	docker run -d --rm \
		--name $(CONTAINER_NAME) \
		-p $(HOST_PORT):$(CONTAINER_PORT) \
		$(IMAGE)

.PHONY: docker-stop
docker-stop:
	-@docker stop $(CONTAINER_NAME) >/dev/null 2>&1 || true

.PHONY: docker-clean
docker-clean: docker-stop
	-@docker rmi $(IMAGE) >/dev/null 2>&1 || true

.PHONY: helm-install
helm-install:
	$(HELM) upgrade --install $(HELM_RELEASE) $(HELM_CHART) \
		--namespace $(HELM_NAMESPACE) \
		--create-namespace \
		$(if $(HELM_VALUES),-f $(HELM_VALUES),)

.PHONY: helm-uninstall
helm-uninstall:
	$(HELM) uninstall $(HELM_RELEASE) --namespace $(HELM_NAMESPACE)
