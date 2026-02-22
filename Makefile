GO ?= go
PKGS := ./...
IMAGE_NAME ?= clock-server
IMAGE_TAG ?= 0.0.1
IMAGE ?= $(IMAGE_NAME):$(IMAGE_TAG)
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

.PHONY: test
test:
	$(GO) test $(PKGS)

.PHONY: build
build:
	$(GO) build $(PKGS)

.PHONY: clean
clean:
	rm -rf bin dist build

.PHONY: docker-build
docker-build:
	docker build -t $(IMAGE) .

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
