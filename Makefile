SHELL := /bin/bash
CONTAINER_NAME=menta2l/k8-injector
IMAGE_TAG?=$(shell git rev-parse HEAD)
KIND_REPO?="kindest/node"
KUBE_VERSION = v1.26.3
KIND_CLUSTER?=cluster1

SRC=$(shell find . -type f -name '*.go' -not -path "./vendor/*")

lint:
	go list ./... | xargs golint -min_confidence 1.0 

vet:
	go vet ./...

test:
	go test ./... -v

tidy:
	go mod tidy

imports:
	goimports -w ${SRC}

clean:
	go clean

build: clean vet lint
	go build -o k8-injector  ./cmd/k8-injector/main.go

release: clean vet lint
	CGO_ENABLED=0 GOOS=linux go build -o k8-injector  ./cmd/k8-injector/main.go

docker:
	docker build --no-cache -t ${CONTAINER_NAME}:${IMAGE_TAG} -t ${CONTAINER_NAME}:latest .

kind-load: docker
	kind load docker-image ${CONTAINER_NAME}:${IMAGE_TAG} --name ${KIND_CLUSTER}

helm-install:
	helm upgrade -i kubernetes-injector ./charts/kubernetes-injector/. --namespace=sidecar-injector --create-namespace --set image.tag=${IMAGE_TAG}

helm-template:
	helm template kubernetes-injector ./charts/kubernetes-injector

kind-create:
	-kind create cluster --image "${KIND_REPO}:${KUBE_VERSION}" --name ${KIND_CLUSTER}

kind-install: kind-load helm-install

kind: kind-create kind-install

follow-logs:
	kubectl logs -n sidecar-injector deployment/kubernetes-injector --follow

install-sample-container:
	helm upgrade -i inject-container ./sample/chart/echo-server/. --namespace=sample --create-namespace

install-sample-init-container:
	helm upgrade -i inject-init-container ./sample/chart/nginx/. --namespace=sample --create-namespace