# InferQuay

InferQuay is a self-hosted AI inference gateway written in Go. It is designed as
a hands-on systems engineering project for learning bounded concurrency,
backpressure, streaming inference, durable job processing, reliability, and
observability on CPU-constrained hardware.

## Project status

The repository is being initialized. Phase 1 will build a small vertical slice:

- a Go HTTP gateway using `net/http`;
- configurable Ollama integration with `qwen2.5:1.5b` as the initial model;
- synchronous and Server-Sent Events streaming inference;
- context cancellation, request timeouts, and bounded concurrency; and
- automated tests backed by a fake model server.

Redis, PostgreSQL, NATS, Kubernetes, and the wider observability stack are
intentionally deferred until the local gateway is working reliably.

## Development principles

- Keep expensive inference work bounded.
- Propagate cancellation and deadlines throughout the request path.
- Prefer focused, idiomatic Go and the standard library.
- Add infrastructure only when it solves a demonstrated problem.
- Measure performance instead of inventing benchmark claims.
- Keep secrets and full user prompts out of source control and logs.

## Roadmap

1. CPU model serving and Go gateway
2. Advanced Go concurrency and backpressure
3. Persistent jobs and distributed infrastructure
4. Observability and reliability
5. Deployment and cloud engineering
6. Benchmarking and security

## License

No license has been selected yet.
