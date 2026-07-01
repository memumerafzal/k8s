# hello-k8s — one-command local Kubernetes demo on kind.
# Run `make` (or `make help`) to see targets.

CLUSTER      ?= hello-k8s
IMAGE        ?= hello-k8s
TAG          ?= dev
OVERLAY      ?= dev
NAMESPACE    ?= hello-$(OVERLAY)
INGRESS_NGINX_VERSION ?= controller-v1.12.0
INGRESS_MANIFEST ?= https://raw.githubusercontent.com/kubernetes/ingress-nginx/$(INGRESS_NGINX_VERSION)/deploy/static/provider/kind/deploy.yaml

.DEFAULT_GOAL := help

## help: show this help
.PHONY: help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | awk -F': ' '{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

## tools: install missing local tooling (kind, kubeconform) via Homebrew
.PHONY: tools
tools:
	@command -v kind >/dev/null || brew install kind
	@command -v kubeconform >/dev/null || brew install kubeconform
	@echo "tools ready"

## cluster: create the kind cluster (idempotent) + install ingress-nginx
.PHONY: cluster
cluster:
	@kind get clusters 2>/dev/null | grep -qx $(CLUSTER) || \
		kind create cluster --name $(CLUSTER) --config kind/cluster.yaml
	@echo "installing ingress-nginx ($(INGRESS_NGINX_VERSION))..."
	@kubectl apply -f $(INGRESS_MANIFEST)
	@echo "waiting for the ingress controller (and its admission webhook) to be ready..."
	@kubectl -n ingress-nginx wait --for=condition=Ready pod \
		--selector=app.kubernetes.io/component=controller --timeout=180s
	@echo "waiting for the admission webhook endpoint to be populated..."
	@for i in $$(seq 1 30); do \
		if [ -n "$$(kubectl -n ingress-nginx get endpoints ingress-nginx-controller-admission -o jsonpath='{.subsets[*].addresses[*].ip}' 2>/dev/null)" ]; then \
			echo "admission webhook endpoint ready"; break; \
		fi; \
		echo "  ...not ready yet ($$i/30)"; sleep 5; \
	done

## test: run the Go unit tests
.PHONY: test
test:
	cd app && go test -race -count=1 ./...

## build: build the app image and load it into the kind cluster
.PHONY: build
build:
	docker build --build-arg VERSION=$(TAG) -t $(IMAGE):$(TAG) ./app
	kind load docker-image $(IMAGE):$(TAG) --name $(CLUSTER)

## deploy: apply the selected overlay (OVERLAY=dev|prod)
.PHONY: deploy
deploy:
	kubectl apply -k deploy/overlays/$(OVERLAY)
	kubectl -n $(NAMESPACE) rollout status deploy/hello-k8s --timeout=120s

## up: full path — cluster + build + deploy the dev overlay
.PHONY: up
up: cluster build deploy
	@echo ""
	@echo "hello-k8s is up. Try:  make demo"

## demo: hit the service through the Ingress and show pod identity
.PHONY: demo
demo:
	@echo "GET http://localhost/version"
	@curl -s http://localhost/version || true
	@echo ""
	@echo "GET http://localhost/  (x3 — watch the count increment)"
	@for i in 1 2 3; do curl -s http://localhost/ | grep -E 'message|pod|count'; echo "--"; done

## forward: port-forward the service to localhost:8080 (Ingress-free fallback)
.PHONY: forward
forward:
	kubectl -n $(NAMESPACE) port-forward svc/hello-k8s 8080:80

## status: show what's running
.PHONY: status
status:
	kubectl -n $(NAMESPACE) get deploy,pods,svc,ingress,hpa,pdb

## lint: render both overlays and validate against the Kubernetes schema
.PHONY: lint
lint:
	@for o in dev prod; do \
		echo "==> validating overlay: $$o"; \
		kubectl kustomize deploy/overlays/$$o | kubeconform -strict -summary \
			-schema-location default \
			-schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json'; \
	done

## render: print the rendered manifests for OVERLAY
.PHONY: render
render:
	kubectl kustomize deploy/overlays/$(OVERLAY)

## metrics: install metrics-server (needed for the prod HPA to scale)
.PHONY: metrics
metrics:
	kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
	kubectl -n kube-system patch deploy metrics-server --type=json \
		-p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'

## down: delete the kind cluster
.PHONY: down
down:
	kind delete cluster --name $(CLUSTER)
