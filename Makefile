build:
	docker build -t redis_local:7.0.4 . -f docker/Dockerfile

start:
	cd backend && docker compose exec backend go run main.go
