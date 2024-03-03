IMAGE_NAME=node-brainer-v0.0.1
CONTAINER_NAME=node-brainer-v0.0.1

build:
	docker build -t $(IMAGE_NAME) .

run:
	docker run -it --name $(CONTAINER_NAME) $(IMAGE_NAME) /bin/sh

run-headless:
	docker run -d --name $(CONTAINER_NAME) $(IMAGE_NAME)

clean:
	docker stop $(CONTAINER_NAME)
	docker rm $(CONTAINER_NAME)

