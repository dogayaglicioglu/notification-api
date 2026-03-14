# Quick Commands

This file keeps the minimum commands to start services, run the load script, and check metrics.

## Ports

- API base URL: `http://localhost:8080`
- Metrics endpoint: `http://localhost:9091/metrics`

## API Endpoints

### Notifications

- `POST /notifications/batch` - Create a batch of notifications.
- `GET /notifications/batch/:batchId` - Get all notifications in one batch.
- `GET /notifications/:id` - Get one notification by ID.
- `PATCH /notifications/cancel` - Cancel all pending notifications.
- `GET /notifications` - List notifications with filters and pagination.

### Service / Docs / Metrics

- `GET /health` - Health check (`http://localhost:8080/health`).
- `GET /metrics` - Prometheus metrics (`http://localhost:9091/metrics`).
- `GET /swagger/*any` - Swagger UI and API docs (`http://localhost:8080/swagger/index.html`).

### Webhook (External Provider)

- Update EXTERNAL_PROVIDER_URL in .env file with your own webhook URL before running.
  or
- Update `EXTERNAL_PROVIDER_URL` in `docker-compose.yml` with your own webhook URL before running.

## Terminal

Run commands directly from terminal.

```bash
docker compose up -d
```

Open Swagger UI in your browser:

```bash
http://localhost:8080/swagger/index.html
```

## Makefile

Same flow with shortcuts.
To run the application, run the following command:

```bash
make build up
```

To test the application, run the following command:

```bash
make load-test
```

To check the metrics, run the following command:

```bash
make metrics
```
