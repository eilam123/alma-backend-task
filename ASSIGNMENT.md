# Analyze Telemetry Data Assignment

**Please read the entire document carefully before starting.**

## Overview

Your goal is to analyze telemetry data that arrived from a customer's production environment.

A **span** represents a single operation or request in a distributed system - it captures metadata about internet-to-service communication, service-to-service communication, database queries, or message queue interactions. The system receives spans from eBPF-based observability agents that are installed on customer Kubernetes clusters.

### Provided Components

**db/memory.go** - An in-memory database with ORM-like operations. Supports table creation, CRUD operations, queries with filters, and JOIN operations. See `db/example_test.go` for usage examples. **Make sure you go over the interfaces in `db/interface.go` to understand available operations for efficient database operations.**

**main.go** - The entry point that calls `createDBSchema()` to initialize the database schema, then processes spans. Implement `createDBSchema` to create the necessary tables.

**backend/processor/processor.go** - The span processor with a `Process` method. This is the entry point where spans arrive for processing.

**backend/api/api.go** - The API backend that should expose `GetCatalog` and `GetConnections`. Implement these methods to return data from the database.

### Example Spans

The following example spans will be used throughout this assignment to illustrate expected behavior:

```json
[
  {
    "id": "span-001",
    "attributes": {
      "ebpf.span.type": "NETWORK",
      "ebpf.source": "internet",
      "ebpf.destination": "mysupermarket-service",
      "ebpf.http.path": "/checkout",
      "ebpf.http.req_body": "{\"user_id\": \"user-789\", \"cart_id\": \"cart-456\", \"credit_card\": {\"number\": \"4532015112830366\", \"expiry\": \"12/25\", \"cvv\": \"123\"}}"
    }
  },
  {
    "id": "span-005",
    "attributes": {
      "ebpf.span.type": "NETWORK",
      "ebpf.source": "mysupermarket-service",
      "ebpf.destination": "users-service",
      "ebpf.http.path": "/users/user-789",
      "ebpf.http.resp_body": "{\"id\": \"user-789\", \"username\": \"johndoe\", \"email\": \"john.doe@email.com\", \"phone\": \"+1-555-123-4567\"}"
    }
  },
  {
    "id": "span-015",
    "attributes": {
      "ebpf.span.type": "MESSAGE",
      "ebpf.source": "mysupermarket-service",
      "ebpf.destination": "kafka",
      "ebpf.queue.topic": "orders-topic",
      "ebpf.action": "queue.produce",
      "ebpf.queue.payload": "{\"order_id\": \"order-101\", \"user_id\": \"user-789\"}"
    }
  },
  {
    "id": "span-003",
    "attributes": {
      "ebpf.span.type": "QUERY",
      "ebpf.source": "users-service",
      "ebpf.destination": "postgres-db",
      "ebpf.db.system": "postgres",
      "ebpf.db.query": "INSERT INTO users (username, email, phone) VALUES ($1, $2, $3)",
      "ebpf.db.query.values": "[\"johndoe\", \"john.doe@email.com\", \"+1-555-123-4567\"]"
    }
  }
]
```

---

## Task 1: App Catalog

Implement support for `GetCatalog` (in `backend/api/api.go`) to return all discovered app items.

### What you need to do:

1. **Analyze and store spans in `Process`** (in `backend/processor/processor.go`) - Parse the incoming spans, extract app item information, and store in the database
2. **Implement `GetCatalog`** (in `backend/api/api.go`) - Retrieve and return the catalog from the database

### Expected Output

The catalog should contain all app items discovered from the spans, including:
- App item name
- App item type (`AppItemType`: SERVICE, DATABASE, QUEUE, INTERNET)
- Components belonging to each app item

### What is an App Item?

An **App Item** represents a discoverable entity in your distributed system. It is identified by its name (extracted from `ebpf.source` or `ebpf.destination` attributes) and has a type:
- **SERVICE** - A microservice (e.g., `order-processor`, `users-service`)
- **DATABASE** - A database instance (e.g., `postgres-db`, `orders-mongo`)
- **QUEUE** - A message broker (e.g., `kafka`)
- **INTERNET** - External traffic coming from outside the cluster (e.g., user browsers, mobile apps)

**How to determine the App Item type:**
- If the source is `internet`, it is INTERNET
- If the span type is `QUERY`, the **destination** is a DATABASE
- If the span type is `MESSAGE`, the **destination** is a QUEUE
- Otherwise, the app item is a SERVICE

### What is a Component?

A **Component** represents a specific interface or operation that belongs to the app item that hosts it. Components always belong to the **destination** app item in a span, as they represent the entry point being accessed on that service.

Component types:
- **EndpointComponent** - An HTTP endpoint (e.g., `GET /users/{id}`, `POST /api/orders`)
- **QueueComponent** - A Kafka topic interaction (e.g., produce to `orders-topic`, consume from `orders-topic`)
- **QueryComponent** - A database operation (e.g., `SELECT` on `users` table, `INSERT` into `orders` table)

**Component Ownership Example:** When `mysupermarket-service` calls `GET /users/user-789` on `users-service`, the `EndpointComponent` for `/users/user-789` belongs to `users-service` (the destination), not to `mysupermarket-service` (the source).

### Example: From Spans to Catalog

Using the example spans from the Overview section:

**Expected Catalog Output:**
```json
{
  "app_items": {
    "internet": {
      "name": "internet",
      "type": "INTERNET",
      "components": []
    },
    "mysupermarket-service": {
      "name": "mysupermarket-service",
      "type": "SERVICE",
      "components": [
        {
          "component_type": "ENDPOINT",
          "path": "/checkout"
        }
      ]
    },
    "users-service": {
      "name": "users-service",
      "type": "SERVICE",
      "components": [
        {
          "component_type": "ENDPOINT",
          "path": "/users/user-789"
        }
      ]
    },
    "kafka": {
      "name": "kafka",
      "type": "QUEUE",
      "components": [
        {
          "component_type": "QUEUE",
          "topic": "orders-topic"
        }
      ]
    },
    "postgres-db": {
      "name": "postgres-db",
      "type": "DATABASE",
      "components": [
        {
          "component_type": "QUERY",
          "query": "INSERT INTO users (username, email, phone) VALUES ($1, $2, $3)"
        }
      ]
    }
  }
}
```

### Guidelines

- Look at the `ebpf.source` and `ebpf.destination` attributes to identify app items
- The `ebpf.span.type` attribute tells you if it's a NETWORK, MESSAGE, or QUERY span

---

## Task 1.1: PII Detection

PII (Personally Identifiable Information) is sensitive data that can identify individuals. You should detect PIIs flowing through the telemetry traffic and associate them with the relevant components.

### PII Ownership

PIIs are always associated with the **component** that exposes them, which belongs to the **destination** app item (the entry point of the service being called).

**PII Ownership Example:** When `mysupermarket-service` calls `GET /users/user-789` on `users-service` and the response contains an EMAIL, the PII belongs to the `/users/user-789` endpoint component, which belongs to `users-service`. The PII does not belong to `mysupermarket-service` (the caller).

### What you need to do:

1. **Investigate the spans** - Explore `data/ebpf_spans.json` to discover what types of sensitive data flow through the system
2. **Define PII types** - Based on your investigation, add a `PIIType` type and constants in `models/models.go`. Then add a `PIIs` field to the component structs
3. **Detect and associate PIIs** - Scan span data for all PII types found and link them to the relevant components
4. **Update `GetCatalog`** - Return components with their PII metadata

### Example: From Spans to PII Detection

Using the example spans from the Overview section:

**Expected Catalog Output with PIIs:**
```json
{
  "app_items": {
    "internet": {
      "name": "internet",
      "type": "INTERNET",
      "components": []
    },
    "mysupermarket-service": {
      "name": "mysupermarket-service",
      "type": "SERVICE",
      "components": [
        {
          "component_type": "ENDPOINT",
          "path": "/checkout",
          "piis": ["CREDIT_CARD"]
        }
      ]
    },
    "users-service": {
      "name": "users-service",
      "type": "SERVICE",
      "components": [
        {
          "component_type": "ENDPOINT",
          "path": "/users/user-789",
          "piis": ["EMAIL", "PHONE"]
        }
      ]
    },
    "kafka": {
      "name": "kafka",
      "type": "QUEUE",
      "components": [
        {
          "component_type": "QUEUE",
          "topic": "orders-topic",
          "piis": []
        }
      ]
    },
    "postgres-db": {
      "name": "postgres-db",
      "type": "DATABASE",
      "components": [
        {
          "component_type": "QUERY",
          "query": "INSERT INTO users (username, email, phone) VALUES ($1, $2, $3)",
          "piis": ["EMAIL", "PHONE"]
        }
      ]
    }
  }
}
```

---

## Task 1.2: App Item Connections

Implement support for `GetConnections` (in `backend/api/api.go`) to return all app-item-to-app-item connections.

### What you need to do:

1. **Extract connections in `Process`** (in `backend/processor/processor.go`) - Identify communications between app items
2. **Implement `GetConnections`** (in `backend/api/api.go`) - Retrieve and return all connections from the database

### Example: From Spans to Connections

Using the example spans from the Overview section:

**Expected Connections Output:**
```json
[
  {
    "source": "internet",
    "destination": "mysupermarket-service",
    "components": [
      {
        "component_type": "ENDPOINT",
        "path": "/checkout"
      }
    ]
  },
  {
    "source": "mysupermarket-service",
    "destination": "users-service",
    "components": [
      {
        "component_type": "ENDPOINT",
        "path": "/users/user-789"
      }
    ]
  },
  {
    "source": "mysupermarket-service",
    "destination": "kafka",
    "components": [
      {
        "component_type": "QUEUE",
        "topic": "orders-topic"
      }
    ]
  },
  {
    "source": "users-service",
    "destination": "postgres-db",
    "components": [
      {
        "component_type": "QUERY",
        "query": "INSERT INTO users (username, email, phone) VALUES ($1, $2, $3)"
      }
    ]
  }
]
```

### Guidelines

- Every span represents a connection from `ebpf.source` to `ebpf.destination`
- Group connections by source + destination, aggregating all components into a list
- The component type depends on the span type (NETWORK → ENDPOINT, MESSAGE → QUEUE, QUERY → QUERY)

---

## Getting Started

You might need to install Go if you don't have it: https://go.dev/doc/install

```bash
cd assignment
go run main.go
go test ./...
```

Run the program to see the current output, then implement the processor and API methods.

---

## Development Principles and Success Criteria

Write production-ready code following these principles:
- **Clean Code** - Write readable, maintainable code with clear naming and logical structure,  Structure your code so new features can be added easily
- **Efficiency** - Optimize for performance db queries
- **Tested** - Write unit tests for your implementation with good coverage
- **Error Handling** - Handle errors gracefully, provide meaningful error messages
