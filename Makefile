run:
	go run cmd/api/main.go

test:
	go test -p 1 ./... -cover

db-up:
	docker-compose up -d

db-down:
	docker-compose down