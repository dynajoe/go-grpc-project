tools:
	./make/tools.sh

generate:
	buf generate

lint:
	go vet ./...
	buf lint

build-docker:
	docker build . -t github.com/dynajoe/go-grpc-template
	
build:
	go build ./...

test-mysql:
	docker run -d --rm -e MYSQL_DATABASE=admin_test \
	--name admin-mysql-test \
	-p 3309:3306 \
	-e MYSQL_USER=root \
	-e MYSQL_ROOT_PASSWORD=notsecret \
	-e MYSQL_ROOT_HOST="%" \
	mariadb:10.5.8

test:
	go test ./...

.PHONY:tools test