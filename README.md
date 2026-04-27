# Scalable Notification Service (Golang)

K3s/Kubernetes орчинд ажиллах, асинхрон worker pool болон provider failover логиктой Notification Service.

## Гол боломжууд

- `POST /v1/notify` endpoint-оор имэйл notification хүлээн авна.
- Хэрэглэгч хүсэлт илгээмэгц `202 Accepted` буцааж, илгээлтийг background queue-д оруулна.
- Go channel + worker pool ашигласан асинхрон гүйцэтгэл.
- Provider Pattern + Failover:
  - Эхний provider quota/rate-limit болсон үед автоматаар дараагийн provider руу шилжинэ.
- Email provider-ууд:
  - `Resend`
  - `Brevo` (Sendinblue)
- Container-ready multi-stage Docker build.
- K3s deployment manifests (`ConfigMap`, `Secret`, `Deployment`, `Service`).

## Архитектурын тойм

Сервис нь clean architecture зарчмаар дараах үүргүүдэд хуваагдсан:

- `cmd/api`:
  - Application bootstrap, config load, HTTP server lifecycle, graceful shutdown.
- `internal/provider`:
  - Channel-agnostic `Notification` interface.
  - `EmailProvider` interface ба provider implementation-ууд (`Resend`, `Brevo`).
- `internal/service`:
  - `FailoverEmailService` (provider chain/failover).
  - `WorkerPool` (queue + background workers).
  - HTTP handler (`/v1/notify`).
- `deployments`:
  - Kubernetes manifests.

## Project бүтэц

```text
.
├── cmd/
│   └── api/
│       └── main.go
├── internal/
│   ├── provider/
│   │   ├── types.go
│   │   ├── resend.go
│   │   └── brevo.go
│   └── service/
│       ├── failover_email_service.go
│       ├── worker_pool.go
│       └── http_handler.go
├── deployments/
│   ├── configmap.yaml
│   ├── secret.yaml
│   ├── deployment.yaml
│   └── service.yaml
├── Dockerfile
├── go.mod
└── go.sum
```

## Interface Design

### 1) Notification (channel-agnostic)

`internal/provider/types.go`:

- `Notification`:
  - `Channel() string`
  - `Validate() error`

Энэ нь цаашид `SMS`, `Slack`, `Push` гэх мэт суваг нэмэхэд domain түвшний contract-ыг нэг мөр болгоно.

### 2) EmailProvider

- `Send(ctx context.Context, message EmailMessage) error`
- `GetName() string`

Provider implementation бүр энэ interface-ийг дагах тул failover service provider-оос үл хамааран ижил логикоор ажиллана.

## Failover логик (Provider Pattern)

`FailoverEmailService` provider-уудыг дарааллаар нь туршина:

1. Эхний provider руу `Send`.
2. Хэрэв quota/rate-limit бол дараагийн provider руу шилжинэ.
3. Хэрэв hard error (quota/rate-limit-аас өөр) бол шууд fail хийнэ.
4. Бүх provider дуусвал нэгдсэн алдаа буцаана.

Failover-д ашиглагдах sentinel errors:

- `ErrRateLimited`
- `ErrQuotaReached`

## Async Processing (Queue + Worker Pool)

`WorkerPool`:

- Buffered channel queue (`QUEUE_SIZE`).
- Configurable worker тоо (`WORKER_COUNT`).
- Request бүрт `request_id` үүсгэнэ.
- Worker бүр send timeout-той (`15s`) ажиллана.

API flow:

1. `/v1/notify` payload validate.
2. Queue-д enqueue.
3. Амжилттай бол `202 Accepted` + `request_id`.
4. Background worker илгээх ажиллагааг гүйцэтгэнэ.

## Email Header Boilerplate (anti-spam best practice)

Outbound email-д дараах header-ууд автоматаар орно:

- `Reply-To`
- `List-Unsubscribe`
- `X-Auto-Response-Suppress`
- `Precedence`

Анхаарах зүйл:

- Production дээр `List-Unsubscribe`-ийн утгыг өөрийн unsubscribe endpoint/domain-д тааруулж шинэчил.
- SPF, DKIM, DMARC-аа DNS дээрээ зөв тохируулах шаардлагатай.

## API Contract

### Health Check

- `GET /healthz`
- Response: `200 OK`, body: `ok`

### Notify Endpoint

- `POST /v1/notify`
- `Content-Type: application/json`
- `Authorization: Bearer <API_KEY>`
- Optional: `X-Trace-Id: <your-trace-id>` (өгөөгүй бол service UUID үүсгэнэ)

#### Request JSON

```json
{
  "to": "user@example.com",
  "subject": "Welcome!",
  "html": "<h1>Hello</h1>",
  "text": "Hello"
}
```

`html` эсвэл `text`-ийн аль нэг нь заавал байх ёстой.

#### Success Response

HTTP `202 Accepted`

```json
{
  "status": "accepted",
  "request_id": "a4af1988-b4b4-4f8b-af7c-130776c6b6bb",
  "trace_id": "9e962e9a-f505-4a18-a2cc-31ec71b5668c"
}
```

#### Error Responses

- `400` invalid payload / validation
- `405` method not allowed
- `429` queue full
- `500` internal enqueue error

Алдааны response-үүд мөн `trace_id` агуулна. API болон worker log-ууд ижил `trace_id`-гаар холбогдож харагдана.

## Environment Variables

| Name | Required | Default | Description |
|---|---|---|---|
| `APP_PORT` | No | `8080` | HTTP порт |
| `WORKER_COUNT` | No | `4` | Worker pool хэмжээ |
| `QUEUE_SIZE` | No | `100` | Queue capacity |
| `API_KEY` | Yes | - | `/v1/notify` endpoint-ийн Bearer auth key |
| `MAIL_FROM` | No | `no-reply@mongols.app` | From email |
| `MAIL_FROM_NAME` | No | `Notification Service` | Brevo sender нэр |
| `RESEND_API_KEY` | Yes* | - | Resend API key |
| `BREVO_API_KEY` | Yes* | - | Brevo API key |
| `RESEND_ENDPOINT` | No | Resend default endpoint | Override endpoint |
| `BREVO_ENDPOINT` | No | Brevo default endpoint | Override endpoint |

`*` Тайлбар: Service асах үед дор хаяж нэг provider (Resend эсвэл Brevo) API key тохирсон байх ёстой.

## Local Run

### 1) Dependencies

- Go `1.26+`

### 2) Env тохируулах

```bash
export APP_PORT=8080
export WORKER_COUNT=4
export QUEUE_SIZE=100
export API_KEY=your_service_api_key
export MAIL_FROM=no-reply@example.com
export MAIL_FROM_NAME="Notification Service"

# At least one provider key is required:
export RESEND_API_KEY=your_resend_key
# export BREVO_API_KEY=your_brevo_key
```

### 3) Run

```bash
go mod tidy
go run ./cmd/api
```

### 4) Test request

```bash
curl -i -X POST http://localhost:8080/v1/notify \
  -H "Authorization: Bearer $API_KEY" \
  -H "X-Trace-Id: demo-trace-001" \
  -H "Content-Type: application/json" \
  -d '{
    "to": "user@example.com",
    "subject": "Test notification",
    "text": "Hello from Notification Service"
  }'
```

`X-Trace-Id` өгөөгүй тохиолдолд API өөрөө UUID үүсгэж response header (`X-Trace-Id`) болон JSON body (`trace_id`) дээр буцаана.

## Docker

### Build image

```bash
docker build -t notification-api:latest .
```

### Run container

```bash
docker run --rm -p 8080:8080 \
  -e RESEND_API_KEY=your_resend_key \
  -e MAIL_FROM=no-reply@example.com \
  notification-api:latest
```

## K3s/Kubernetes Deployment

Manifest-ууд `deployments/` дотор бэлэн:

- `configmap.yaml`
- `secret.yaml`
- `deployment.yaml`
- `service.yaml`

### 1) Secret update хийх

`deployments/secret.yaml` дотор API key-үүдийг бодит утгаар солино.

### 2) Apply manifests

```bash
kubectl apply -f deployments/configmap.yaml
kubectl apply -f deployments/secret.yaml
kubectl apply -f deployments/deployment.yaml
kubectl apply -f deployments/service.yaml
```

### 3) Шалгах

```bash
kubectl get pods
kubectl get svc notification-api
kubectl logs deploy/notification-api
```

Service нь `ClusterIP` тул cluster дотроос хандах эсвэл Ingress нэмэх шаардлагатай.

## Resource & Reliability

`Deployment` дээр:

- `replicas: 2`
- Requests:
  - CPU: `100m`
  - Memory: `128Mi`
- Limits:
  - CPU: `500m`
  - Memory: `512Mi`
- `livenessProbe` + `readinessProbe`: `/healthz`

## Түгээмэл асуудал (Troubleshooting)

### Service асахгүй, "at least one provider is required"

- `RESEND_API_KEY` эсвэл `BREVO_API_KEY` тохируулаагүй байна.

### `/v1/notify` дээр `429`

- Queue дүүрсэн. `QUEUE_SIZE` эсвэл `WORKER_COUNT`-ийг өсгө.
- Provider API удаашралтай/лимиттэй үед queue backlog үүсч болно.

### Mail spam-д орох

- Sender domain дээр SPF/DKIM/DMARC шалга.
- `MAIL_FROM` нь provider дээр verified domain байх шаардлагатай.

## Production-д санал болгох дараагийн алхмууд

- Retry + exponential backoff policy.
- Dead-letter queue.
- Structured logging (`zap` / `zerolog`).
- Metrics (`Prometheus`) болон tracing.
- Request status persistence (DB/Redis) + callback/webhook.

