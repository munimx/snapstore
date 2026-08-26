# SnapStore

A Go service that proves an object-storage binding is usable, not merely
present. Deployed with [Liftoff](https://tryliftoff.tech).

| Method | Path              | Description                                     |
|--------|-------------------|-------------------------------------------------|
| GET    | `/health`         | Health check                                    |
| GET    | `/api/bindings`   | Which storage variables reached the container   |
| GET    | `/api/roundtrip`  | Write a blob, read it back, delete, confirm gone|
| GET    | `/api/blobs`      | List the bound container                        |

Secrets are reported by length only, never by value.
