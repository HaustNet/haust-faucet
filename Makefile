.PHONY: docker-build docker-push

VERSION=1.3.0
DOCKER_IMAGE=ghcr.io/haustnet/haust-faucet:$(VERSION)
DOCKER_CONTAINER_NAME=haust-faucet

docker-build:
	docker build \
		--label "org.opencontainers.image.source=https://github.com/HaustNet/haust-faucet" \
		-t $(DOCKER_IMAGE) .

docker-push:
	docker push $(DOCKER_IMAGE)

logs:
	docker logs -f $(DOCKER_CONTAINER_NAME)

stop:
	docker stop $(DOCKER_CONTAINER_NAME) && docker rm $(DOCKER_CONTAINER_NAME)
