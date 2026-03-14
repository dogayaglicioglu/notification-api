.PHONY: help up down restart logs load-test metrics

help:
	@echo "Available commands:"
	@echo "  make up         # Start services"
	@echo "  make down       # Stop services"
	@echo "  make restart    # Restart services"
	@echo "  make logs       # Tail logs"
	@echo "  make load-test  # Run test-100k-notifications.sh"
	@echo "  make metrics    # Show queue/worker metrics"

build:
	docker compose build
up:
	docker compose up -d

down:
	docker compose down

restart: down build up

logs:
	docker compose logs -f

load-test:
	./test-100k-notifications.sh

metrics:
	curl -s http://localhost:9091/metrics | grep -E 'notification_queue_size|notifications_processed_total|notification_processing_duration_seconds|notification_retries_total|notification_active_workers'
