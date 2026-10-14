# FluxQuay — Complete Project Context and Development Instructions

## 1. Project Identity

**Name:** FluxQuay

**Description:** A self-hosted AI inference gateway built in Go, featuring concurrent request processing, distributed rate limiting, durable job queues, observability, and fault-tolerant model serving.

**Category:** Backend Engineering, Distributed Systems, AI Infrastructure, Cloud Engineering.

**Primary language:** Go.

**Project type:** Portfolio-quality systems engineering project with a working implementation, automated tests, documented architectural decisions, and reproducible performance experiments.

---

## 2. Background and Motivation

I am a Computer Science student developing my skills in Go backend engineering, concurrency, distributed systems, and cloud infrastructure.

I have previously built a backend application called **TaskFlow**.

Repository: https://github.com/SaadShakeel1/TaskFlow

TaskFlow gave me experience with:

- Go REST API development using `net/http`.
- PostgreSQL and SQL queries.
- Database migrations.
- Authentication and authorization.
- Role-based access control.
- API middleware.
- Transactions and optimistic concurrency control.
- Data validation and error handling.
- Background processing.
- Goroutines, channels, and worker pools.
- API and database testing.
- Structured backend architecture.

The agent should inspect TaskFlow when useful to understand my existing experience rather than assuming every listed concept was implemented to production depth.

**FluxQuay must go beyond a conventional CRUD backend.**

Its purpose is to teach infrastructure-oriented concepts that are not yet deeply explored in TaskFlow, especially:

1. Advanced Go concurrency and synchronization.
2. Backpressure and resource management.
3. Distributed rate limiting.
4. Message brokers and durable asynchronous processing.
5. Reliability and fault tolerance.
6. Model serving and inference orchestration.
7. Caching and performance optimization.
8. Observability and distributed tracing.
9. Load testing and profiling.
10. Docker, CI/CD, and cloud deployment.
11. Practical security engineering.
12. Scalability and distributed systems tradeoffs.

I want to implement these concepts myself where doing so is educational, rather than depending on frameworks that hide their behavior.

---

## 3. Hardware and Resource Constraints

The project must be runnable locally on my existing laptop.

**System specifications:**

- CPU: Intel Core i7-1165G7, 11th generation.
- CPU architecture: x86-64.
- Cores/threads: 4 cores, 8 threads.
- RAM: 32 GB.
- GPU: Intel Iris Xe integrated graphics.
- Dedicated NVIDIA GPU: None.
- Total storage: Approximately 477 GB.
- Available storage at project planning time: Approximately 101 GB.
- Primary operating system: Windows 11.
- Development environment: Windows, WSL2/Linux, Go, and Docker.

### Important constraints

1. The project must support CPU-only inference.
2. GPU acceleration must not be required.
3. Local development should not require paid cloud services.
4. Avoid unnecessarily large language models.
5. Avoid loading multiple model instances unless intentionally benchmarking them.
6. Keep local infrastructure memory usage reasonable.
7. Support running infrastructure services independently rather than requiring the entire stack for basic development.
8. Make resource limits configurable.
9. Assume inference throughput is limited by CPU capacity.
10. Design concurrency around those resource limits instead of assuming unlimited compute.

### Initial model choice

Start with a small quantized model:

**Qwen2.5 1.5B through Ollama.**

Later, optionally benchmark Qwen3 0.6B, 1.7B, or 4B variants.

Model selection must remain configurable.

Ollama is the initial inference engine, but the architecture should support replacing it with llama.cpp or another compatible model server later.

**Do not implement an LLM inference engine from scratch.**

The project concerns serving, orchestration, concurrency, reliability, and infrastructure.

---

## 4. Core Project Objective

Build a Go service that accepts authenticated inference requests, schedules them safely, invokes a locally hosted language model, streams or stores results, and provides operational visibility.

The system should remain predictable when:

- Many clients connect simultaneously.
- More requests arrive than the model can process.
- A client disconnects.
- A request times out.
- A model inference becomes slow.
- A background worker crashes.
- Redis or another dependency fails.
- The service receives malformed requests.
- The queue becomes saturated.
- The server receives a shutdown signal.

The central engineering question is:

**How do we build a reliable, observable, resource-bounded AI inference system on limited hardware?**

This question should guide architectural decisions throughout development.

---

## 5. Proposed Technology Stack

| Component | Technology |
|---|---|
| Backend | Go, standard `net/http` |
| Inference engine | Ollama initially |
| Database | PostgreSQL |
| Caching | Redis |
| Distributed rate limiting | Redis |
| Message broker | NATS JetStream |
| In-process background execution | Go goroutines and channels |
| Telemetry instrumentation | OpenTelemetry |
| Metrics | Prometheus |
| Dashboards | Grafana |
| Distributed tracing | Jaeger |
| Reverse proxy | Caddy or Nginx |
| Containers | Docker |
| Local infrastructure | Docker Compose |
| Container orchestration | Kubernetes, later |
| CI/CD | GitHub Actions |
| Infrastructure as Code | Terraform, later |
| Load testing | k6 |
| Profiling | Go pprof |
| Security testing | Go fuzzing and OWASP ZAP |

### Technology-selection principles

- Prefer Go's standard library for HTTP handling.
- Use focused, established dependencies where they solve a real problem.
- Do not introduce microservices unnecessarily.
- Begin with a modular monolith for the gateway.
- Run the model server as a separate process.
- Introduce distributed infrastructure only when the previous phase is functioning.
- Keep the code readable, testable, and idiomatic.
- Explain important technology choices and their alternatives.

---

## 6. High-Level Architecture

```text
                      Clients
                         |
                         v
                   HTTPS Proxy
                         |
                         v
                  Load Balancer
                         |
                +--------+--------+
                |                 |
           Go Gateway A      Go Gateway B
                |                 |
                +--------+--------+
                         |
               Authentication
               Request Validation
               Distributed Limits
                         |
                +--------+--------+
                |                 |
                v                 v
          Sync Inference      Async Inference
                |                 |
         Concurrency Limit    NATS JetStream
                |                 |
                |              Go Workers
                |                 |
                +--------+--------+
                         |
                         v
                 Model Runtime
                     Ollama
                         |
                         v
                Local CPU Inference

Supporting Services:

Redis
 - Distributed rate limits
 - Caching
 - Shared quota state

PostgreSQL
 - API key metadata
 - Job metadata
 - Job status and results
 - Usage records

OpenTelemetry
 - Trace propagation
 - Request spans
 - Inference spans
 - Worker spans

Prometheus
 - Metrics collection

Grafana
 - Operational dashboards

Jaeger
 - Distributed trace inspection
```

This is the target architecture, not the initial implementation.

Initially run a single Go gateway and one Ollama model server.

Later introduce distributed API instances, durable messaging, and shared infrastructure.

**The API gateway and model server are separate components.**

Go handles request orchestration, networking, authentication, concurrency control, and infrastructure behavior.

Ollama handles neural-network inference.

---

## 7. Core Functional Requirements

### 7.1 Synchronous inference

Implement an endpoint that:

1. Accepts a prompt and inference parameters.
2. Validates the request.
3. Authenticates the caller.
4. Applies relevant quotas.
5. Acquires limited inference capacity.
6. Calls the model server.
7. Returns the generated output.
8. Records duration, status, and errors.
9. Releases capacity correctly.

The model must not receive unrestricted concurrent requests.

### 7.2 Streaming inference

Support Server-Sent Events (SSE).

Requirements:

- Stream generated tokens incrementally.
- Properly flush response data.
- Detect client disconnection.
- Propagate cancellation upstream.
- Stop unnecessary processing.
- Handle errors after streaming begins.
- Respect request deadlines.
- Avoid leaking goroutines or network connections.

Use the appropriate HTTP streaming headers.

Translate Ollama's streaming protocol into a clearly documented client-facing SSE format.

### 7.3 Asynchronous inference

Support longer-running jobs.

Expected flow:

1. Client submits a job.
2. Gateway validates and persists the request.
3. A durable job message is published.
4. Client receives a job identifier.
5. A worker claims the job.
6. Worker performs inference.
7. Results and status are persisted.
8. Client retrieves the result.

Include states such as:

- Pending
- Running
- Completed
- Failed
- Cancelled

Document valid state transitions.

A job must not silently disappear when an API process restarts.

### 7.4 Authentication and quotas

Support API key authentication.

Requirements:

- Secure random API keys.
- Store key hashes rather than plaintext secrets.
- Revoke keys.
- Associate keys with users or tenants.
- Apply per-key or per-tenant rate limits.
- Support configurable usage quotas.
- Prevent unauthorized access to another user's jobs.

JWT authentication may be added later if it provides a meaningful use case.

Avoid developing a complete user-management product unnecessarily.

---

## 8. Advanced Go Concurrency Requirements

This is one of the most important educational parts of FluxQuay.

I want practical experience with the following concepts.

| Concept | Intended application |
|---|---|
| Goroutines | HTTP requests, workers, background operations |
| Channels | In-process job dispatch and coordination |
| Buffered channels | Bounded local job queues |
| Unbuffered channels | Synchronization where appropriate |
| `sync.WaitGroup` | Worker lifecycle and shutdown |
| `sync.Mutex` | Shared mutable state |
| `sync.RWMutex` | Read-heavy synchronized state |
| `sync.Once` | One-time initialization |
| `sync/atomic` | Simple concurrent state/counters |
| `context.Context` | Cancellation and deadlines |
| `select` | Jobs, timeouts, shutdown signals |
| Semaphores | Restrict active inference operations |
| `errgroup` | Coordinate related concurrent tasks |
| Worker pools | Bounded background execution |
| Race detector | Detect synchronization errors |
| pprof | Investigate CPU, memory, and goroutines |

### Worker pool behavior

Implement a bounded worker pool.

The number of workers must be configurable.

Avoid creating unlimited goroutines for expensive inference requests.

When capacity is exhausted:

- Requests may wait in a bounded queue.
- Excess requests must be rejected predictably.
- Client cancellations should remove or invalidate unnecessary work.
- Shutdown must not leave workers running indefinitely.

### Backpressure

Backpressure is a major learning objective.

Explore:

- Queue capacity.
- Admission control.
- Inference concurrency.
- Request timeouts.
- Slow clients.
- Resource saturation.
- Queue waiting time.
- Fairness between clients.

Return appropriate HTTP errors such as 429 for quota/rate-limit violations and 503 for temporary capacity exhaustion.

### Cancellation

Every inference request should support cancellation.

Use request contexts and propagate cancellation to the model-server HTTP request.

Handle:

- Client disconnects.
- Request deadlines.
- Server shutdown.
- Explicit asynchronous job cancellation.

Distinguish between cancelling Go's HTTP request and the actual model runtime stopping inference. Verify the runtime's behavior experimentally.

### Graceful shutdown

When the service receives a termination signal:

1. Stop accepting new work.
2. Cancel or drain work according to documented policy.
3. Wait for workers within a bounded deadline.
4. Release resources.
5. Close database and broker connections.
6. Exit cleanly.

Avoid goroutine leaks, deadlocks, and indefinite waiting.

---

## 9. Distributed Systems Requirements

Introduce distributed concepts after the local concurrency implementation is working.

### Distributed rate limiting

Implement Redis-backed limiting.

Explore:

- Token bucket or sliding-window approaches.
- Atomic Redis operations.
- Shared limits across gateway instances.
- Race conditions between concurrent requests.
- Rate-limit headers.
- Redis outage behavior.

Document whether the system fails open or fails closed when Redis is unavailable, and why.

### Durable message queues

Use NATS JetStream for asynchronous inference.

Requirements:

- Persistent job delivery.
- Explicit acknowledgments.
- Retry handling.
- Backoff.
- Dead-letter handling.
- Duplicate delivery protection.
- Idempotent job processing.
- Crash recovery.

Understand at-least-once delivery semantics.

Do not assume exactly-once execution simply because a message broker is used.

### Distributed coordination

Learn how multiple gateway instances coordinate without relying on shared in-process memory.

Examples:

- Shared rate limits.
- Shared job state.
- Distributed worker competition.
- Duplicate jobs.
- Worker crashes.
- Resource ownership.

The model runtime is still CPU-constrained, so horizontal scaling of gateway instances does not automatically increase inference throughput.

---

## 10. Caching

Implement caching only where it is semantically appropriate.

Potential features:

- Redis response caching.
- Cache keys based on model, prompt, and generation parameters.
- TTL.
- Expiration.
- Invalidation.
- Cache hit/miss metrics.
- Cache stampede prevention.

For nondeterministic generation, document when cached responses are acceptable.

Do not assume that two requests with the same prompt necessarily require identical generated output.

Measure caching improvements rather than assuming them.

---

## 11. Reliability and Fault Tolerance

Implement and understand:

- Timeouts.
- Context propagation.
- Retries with exponential backoff.
- Jitter.
- Circuit breakers.
- Dependency health checks.
- Readiness probes.
- Graceful shutdown.
- Overload rejection.
- Bounded queues.
- Worker crash recovery.
- Idempotent job execution.
- Partial failure handling.

Avoid retries that unintentionally multiply expensive model requests.

Use retries only when the operation is safe and a retry is justified.

---

## 12. Observability

Build a system that allows me to answer:

- Why was this request slow?
- Was latency caused by queue waiting or inference?
- How many requests are active?
- How many requests failed?
- Is the CPU saturated?
- Are workers keeping up?
- Is the model server healthy?
- Are requests being rejected?
- Are cancellations working?
- Are cache hits improving throughput?

### Logging

Use structured JSON logs.

Include relevant identifiers such as:

- Request ID.
- Job ID.
- Trace ID.
- Operation.
- Duration.
- Status.
- Error category.

Do not log API secrets or full user prompts by default.

### Metrics

Measure:

- Total HTTP requests.
- Requests per second.
- Error rate.
- p50 latency.
- p95 latency.
- p99 latency.
- Active inference requests.
- Model inference duration.
- Time to first token.
- Tokens generated per second.
- Queue depth.
- Queue waiting time.
- Retry counts.
- Request cancellations.
- Cache hit ratio.
- Goroutine count.
- CPU utilization.
- Memory utilization.

Avoid high-cardinality metric labels such as raw user IDs or request IDs.

### Distributed tracing

Use OpenTelemetry.

Instrument:

- HTTP request handling.
- Authentication.
- Rate limiting.
- Cache access.
- Job publication.
- Queue waiting where measurable.
- Worker processing.
- Calls to Ollama.
- Database operations.

Use Jaeger to inspect traces across components.

### Profiling

Use Go pprof to investigate:

- CPU hotspots.
- Memory allocations.
- Goroutine leaks.
- Blocking.
- Mutex contention.

Profiling should help guide performance improvements.

---

## 13. API Design

Initial target endpoints:

| Method | Endpoint | Purpose |
|---|---|---|
| POST | `/v1/inference` | Synchronous inference |
| POST | `/v1/inference/stream` | Streaming inference |
| POST | `/v1/jobs` | Submit async job |
| GET | `/v1/jobs/{id}` | Job status and result |
| DELETE | `/v1/jobs/{id}` | Request cancellation |
| GET | `/healthz` | Liveness |
| GET | `/readyz` | Readiness |
| GET | `/metrics` | Prometheus metrics |

Only implement endpoints relevant to the current development phase.

Use consistent response structures and error formats.

Support:

- Request validation.
- Reasonable body size limits.
- Timeouts.
- Context cancellation.
- Clear HTTP status codes.
- API versioning.

Protect metrics and administrative surfaces from public access.

---

## 14. Docker and Local Development

The system should support local development without requiring a cloud provider.

### Initial setup

Run:

- Go gateway locally.
- Ollama locally.
- Small quantized model on CPU.

Ollama should normally listen on a local interface, not be exposed publicly.

Make the model server base URL configurable through an environment variable such as `OLLAMA_BASE_URL`.

### Later setup

Use Docker Compose for:

- Gateway.
- PostgreSQL.
- Redis.
- NATS.
- Prometheus.
- Grafana.
- Jaeger.

Ollama can remain on the host initially.

Document how containers reach a model server running on Windows or WSL2.

Avoid hardcoded networking assumptions.

Do not require every infrastructure service to be running during Phase 1.

---

## 15. Testing Requirements

Testing is a core feature, not an afterthought.

### Unit tests

Test:

- Request validation.
- Rate-limit calculations.
- State transitions.
- Queue admission.
- Worker coordination.
- Cancellation behavior.
- Error handling.
- Configuration loading.

### Integration tests

Test:

- HTTP API behavior.
- Ollama adapter using a controlled fake server.
- Redis behavior.
- PostgreSQL persistence.
- Message delivery.
- Retry processing.
- Worker crash recovery.

Avoid making ordinary unit tests depend on downloading or executing a real AI model.

### Concurrency tests

Test:

- Simultaneous requests.
- Queue saturation.
- Channel closure.
- Concurrent shutdown.
- Cancellation races.
- Worker termination.
- Duplicate job processing.

Run the race detector where applicable.

### Load testing

Use k6.

Run experiments with increasing concurrency:

- Single user.
- Several concurrent users.
- Gradually increasing workload.
- Sustained workload.
- Burst workload.

Measure throughput, latency, errors, and resource consumption.

Use fixed prompt sizes, output token limits, and comparable test configurations.

### Failure injection

In an isolated local environment:

- Stop the model server.
- Kill a worker.
- Restart the gateway.
- Stop Redis.
- Restart the message broker.
- Introduce artificial delays.
- Submit malformed requests.
- Submit oversized requests.
- Disconnect streaming clients.
- Saturate the request queue.

Record expected and observed behavior.

### Security testing

Perform authorized testing only against environments I own or control.

Test:

- Invalid authentication.
- Expired or revoked keys.
- Cross-user job access.
- Oversized payloads.
- Rate-limit bypass attempts.
- Malformed JSON.
- Slow clients.
- Resource exhaustion.
- Input fuzzing.

Use OWASP guidance where appropriate.

---

## 16. Performance Engineering

The project should demonstrate measurable engineering improvements.

Build reproducible benchmarks comparing:

- Unbounded vs bounded inference concurrency.
- Different worker pool sizes.
- Different queue capacities.
- Different request timeouts.
- Caching enabled vs disabled.
- Single API instance vs multiple API instances.
- Different quantized model sizes.
- Different prompt and generation lengths.

Document:

- Test environment.
- Model and quantization.
- Worker count.
- Queue size.
- Concurrent clients.
- Request duration.
- Throughput.
- Tail latency.
- Error rate.
- Memory consumption.
- CPU consumption.

Do not invent benchmark numbers or performance improvements.

Measurements must come from actual experiments.

Explain why a configuration performs better or worse.

---

## 17. Cloud and DevOps

After the local service becomes reliable:

### Docker

Learn and implement:

- Multi-stage builds.
- Small production images.
- Container networking.
- Volumes.
- Health checks.
- Environment configuration.
- Resource limits.

### CI/CD

Use GitHub Actions.

Pipeline should eventually cover:

- Formatting checks.
- Static analysis.
- Unit tests.
- Race tests.
- Integration tests where practical.
- Security checks.
- Container builds.

### Kubernetes

Add later as a separate learning phase.

Explore:

- Pods.
- Deployments.
- Services.
- Ingress.
- ConfigMaps.
- Secrets.
- Readiness/liveness probes.
- Resource requests and limits.
- Horizontal Pod Autoscaling.

Do not introduce Kubernetes before the application runs reliably under Docker Compose.

### Cloud

Optionally deploy on a cloud VM.

Cloud deployment should cover:

- Linux administration.
- Firewall configuration.
- HTTPS.
- Reverse proxies.
- DNS.
- Service configuration.
- Secure secrets.
- Basic infrastructure monitoring.

Terraform can be introduced to make infrastructure reproducible.

Keep costs low and do not provision paid infrastructure without explicit approval.

---

## 18. Development Roadmap

### Phase 1 — CPU Model Serving and Go Gateway

**Goal:** Make the smallest functional vertical slice.

Tasks:

- Initialize the Go repository.
- Establish the backend structure.
- Add configuration management.
- Integrate Ollama.
- Run Qwen2.5 1.5B locally.
- Implement synchronous inference.
- Implement streaming inference using SSE.
- Add request validation.
- Add context cancellation.
- Add request timeouts.
- Add bounded inference concurrency.
- Add basic structured logging.
- Add health/readiness endpoints.
- Write tests using a mock/fake model server.
- Document local setup.

**Done when:** A client can send requests to the Go gateway, obtain generated text and streamed tokens, and the gateway handles timeouts, cancellations, and overload predictably.

### Phase 2 — Advanced Go Concurrency

**Goal:** Understand concurrency deeply.

Tasks:

- Build an in-process worker pool.
- Implement bounded channels.
- Introduce a local asynchronous queue.
- Coordinate workers using WaitGroups.
- Explore mutexes and atomic counters where needed.
- Implement `select`-based cancellation/shutdown.
- Add backpressure.
- Add concurrent request tests.
- Use the race detector.
- Investigate goroutine leaks.

**Done when:** The worker system handles concurrency safely, rejects excessive work, and shuts down cleanly.

This phase is educational; the in-memory queue is not expected to provide durability.

### Phase 3 — Persistent Jobs and Distributed Infrastructure

**Goal:** Move beyond a single-process backend.

Tasks:

- Introduce PostgreSQL job persistence.
- Introduce NATS JetStream.
- Build separate background workers.
- Add retries and dead-letter handling.
- Add idempotency.
- Introduce Redis.
- Implement distributed rate limiting.
- Support multiple gateway instances.
- Handle dependency failures.

**Done when:** Jobs survive gateway restarts, multiple gateway instances coordinate correctly, and failure recovery is demonstrated.

### Phase 4 — Observability and Reliability

**Goal:** Understand what happens inside the system under real workloads.

Tasks:

- Add OpenTelemetry.
- Add Prometheus metrics.
- Add Grafana dashboards.
- Add Jaeger tracing.
- Add CPU and memory profiling.
- Add circuit breakers and retry policies.
- Test cancellation propagation.
- Run failure-injection experiments.

**Done when:** We can diagnose latency bottlenecks, resource saturation, and dependency failures using real telemetry.

### Phase 5 — Deployment and Cloud Engineering

**Goal:** Make the application deployable and reproducible.

Tasks:

- Containerize the gateway.
- Create Docker Compose configuration.
- Add CI/CD.
- Configure HTTPS and reverse proxy.
- Deploy on Linux.
- Explore Kubernetes.
- Explore Terraform.

**Done when:** Another developer can reproduce the environment and deploy the application from documented instructions.

### Phase 6 — Benchmarking and Security

**Goal:** Validate the system under challenging conditions.

Tasks:

- Build k6 workload scenarios.
- Benchmark different concurrency limits.
- Measure p95 and p99 latency.
- Test burst traffic.
- Test queue saturation.
- Test worker failures.
- Perform security tests.
- Fix identified bottlenecks.
- Produce a performance and reliability report.

**Done when:** The repository contains evidence of the system's behavior, limitations, and improvements.

---

## 19. Suggested Repository Structure

The structure can evolve, but prefer something like:

```text
fluxquay/
├── cmd/
│   ├── gateway/
│   │   └── main.go
│   └── worker/
│       └── main.go
│
├── internal/
│   ├── api/
│   ├── config/
│   ├── inference/
│   ├── auth/
│   ├── ratelimit/
│   ├── queue/
│   ├── worker/
│   ├── cache/
│   ├── storage/
│   ├── telemetry/
│   └── middleware/
│
├── migrations/
├── tests/
├── benchmarks/
├── infra/
│   ├── docker/
│   ├── observability/
│   └── kubernetes/
│
├── docs/
│   ├── architecture/
│   ├── decisions/
│   └── experiments/
│
├── scripts/
├── .github/
│   └── workflows/
│
├── docker-compose.yaml
├── .env.example
├── go.mod
├── README.md
└── AGENTS.md
```

Do not create empty packages or placeholder services just to match this structure.

Add directories as their functionality is implemented.

Use `internal` packages to maintain clear boundaries.

Avoid unnecessary interfaces and premature abstractions.

---

## 20. Code Quality Requirements

Follow these principles:

1. Write idiomatic Go.
2. Prefer simple designs over unnecessary abstractions.
3. Use the standard library where practical.
4. Keep packages cohesive.
5. Pass `context.Context` through request-scoped operations.
6. Avoid global mutable state.
7. Handle errors explicitly.
8. Use structured logging.
9. Write tests alongside functionality.
10. Never ignore errors silently.
11. Use bounded concurrency for expensive operations.
12. Avoid goroutine leaks.
13. Avoid unbounded queues.
14. Make resource limits configurable.
15. Keep secrets out of source control.
16. Document non-obvious concurrency decisions.
17. Prefer explicit dependencies and dependency injection.
18. Keep external infrastructure replaceable behind sensible boundaries.
19. Avoid premature optimization.
20. Benchmark before claiming performance improvements.

Do not add frameworks, distributed services, or patterns simply because they sound production-grade.

Every component should solve a clear problem.

---

## 21. Educational Requirements for the Coding Agent

**Important: This is also a learning project.**

I want to understand the engineering, not merely receive working code.

When implementing an important concept:

- Explain the problem it solves.
- Explain why the selected approach fits this project.
- Identify the Go or distributed systems concepts involved.
- Discuss important tradeoffs.
- Explain how the implementation works.
- Highlight concurrency hazards.
- Explain how to verify correctness.
- Provide commands to run and test the feature.

For significant Go concurrency code, explain the behavior of goroutines, channels, locks, contexts, and shutdown coordination.

Avoid generating large amounts of complicated code without explaining the architecture.

Prefer incremental, understandable implementation.

When meaningful alternatives exist, explain them briefly and choose one.

Do not rewrite already-working code unnecessarily.

---

## 22. Working Instructions for the Repository Agent

Before making changes:

1. Inspect the existing repository.
2. Identify what is already implemented.
3. Check the Go module, configuration, and current tests.
4. Avoid overwriting working code.
5. Identify the current development phase.
6. Present a short implementation plan.
7. Implement only the next logical vertical slice.
8. Add tests.
9. Run relevant checks.
10. Summarize what changed and what remains.

When writing code:

- Make focused changes.
- Prefer maintainability.
- Keep functions and packages understandable.
- Explain complex behavior.
- Do not introduce unused infrastructure.
- Do not create fake performance results.
- Do not claim tests passed unless executed.
- Do not commit secrets.
- Do not push changes, publish packages, expose services publicly, or provision cloud resources without explicit approval.

For every completed step, report:

**Implemented:** What was added.

**Concepts learned:** The relevant backend/Go concepts.

**Design decisions:** Why this implementation was selected.

**Verification:** Tests and commands actually run.

**Next step:** The smallest logical next feature.

---

## 23. Definition of Project Success

FluxQuay is successful when it demonstrates:

- A working CPU-hosted LLM behind a Go API.
- Safe concurrent request handling.
- Streaming inference.
- Bounded inference execution.
- Predictable backpressure.
- Correct cancellation behavior.
- Durable background inference jobs.
- Shared quotas across gateway instances.
- Reliable worker coordination.
- Meaningful caching where appropriate.
- Distributed tracing and operational metrics.
- Graceful recovery from tested failures.
- Automated tests and CI/CD.
- Reproducible deployment.
- Measured load-testing results.
- Documented architecture and tradeoffs.

The project does not need to outperform commercial inference platforms.

The goal is to demonstrate sound systems engineering on constrained hardware.

---

## 24. Concepts Outside the Initial Scope

Do not attempt to implement everything in computer science.

Topics that can remain future work:

- Distributed consensus algorithms.
- Raft implementation.
- Custom database engines.
- Database sharding.
- Multi-region deployment.
- GPU kernel programming.
- Training large language models.
- Building transformer architectures from scratch.
- Custom container orchestration.
- Implementing a full message broker.

We can study these later, but they should not distract from the core project.

---

## 25. Immediate Instructions

**Begin with Phase 1 only.**

First inspect the current FluxQuay repository.

If it is empty, propose a minimal initial Go project structure and begin building the first vertical slice.

The first milestone is:

**A working Go HTTP gateway that connects to a locally hosted Ollama model running on CPU, supports synchronous and streaming inference, propagates cancellation, applies bounded concurrency, and has automated tests.**

Do not add Redis, PostgreSQL, NATS, Kubernetes, or the entire observability stack during the first milestone.

Make inference configurable through environment variables.

Assume Ollama is running locally and the initial model is `qwen2.5:1.5b`.

Design the model interaction behind a small, testable interface without unnecessary abstraction.

Use Go `net/http`.

Prioritize correctness, learning, and testability.

After implementing the first milestone, provide:

1. A summary of the architecture.
2. The files created or modified.
3. Instructions for running Ollama and FluxQuay.
4. Commands for testing the API.
5. Explanation of how concurrency limits and cancellation work.
6. Tests performed and their results.
7. Known limitations.
8. The proposed next milestone.

**Final guiding principle:**

FluxQuay should teach how to build, operate, secure, observe, and scale a reliable inference gateway—not merely how to call a language model through an HTTP API.

Build incrementally, measure behavior, explain engineering tradeoffs, and prioritize deep understanding of Go backend and distributed systems concepts.
