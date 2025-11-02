# Cloud Native Encabulator - Product Overview

## What It Is

A fully functional Kubernetes microservices platform that demonstrates modern cloud-native patterns using humorous "Turbo Encabulator" naming conventions. While the service names are whimsical (Hydrocoptic Data Store, Panametric Fam Processor), the implementation is production-quality.

## Purpose

Educational demonstration platform for:
- Kubernetes beginners and intermediate users
- DevOps engineers learning microservices patterns
- Product managers understanding cloud-native architecture
- Technical educators teaching distributed systems

## Core Concept: Encabulations

An **encabulation** is the core data processing unit - a request that flows through the entire architecture demonstrating real distributed system behavior with actual data persistence, caching, and metrics collection.

## Architecture

**13 microservices** organized into 4 planes:

- **Data Plane** (3 services): PostgreSQL, Redis, InfluxDB wrappers
- **Processing Plane** (3 services): Core logic, message broker, API gateway
- **Support Plane** (3 services): Feature flags, auth, telemetry
- **Control Plane** (4 services): Interactive UI, chaos engineering, traffic generation, demo orchestration

## Key Features

- Real-time system topology visualization with animated traffic flow
- Interactive traffic control (0.5-1000 TPS, latency injection, payload complexity)
- Complete observability with Prometheus, Grafana, and custom metrics
- Chaos engineering capabilities for resilience testing
- GitOps deployment with ArgoCD
- Auto-scaling demonstrations (HPA)

## Technology Stack

Polyglot monorepo: Go (7 services), TypeScript/Node.js (3 services), Python (3 services)

## Deployment Evolution

The project demonstrates deployment strategy evolution:
- Phase 1: Helm-first development (preserved in `helm-first` branch)
- Phase 2: GitOps with ArgoCD (current on `main`)
- Phase 3: AWS managed services with ACK (planned)
- Phase 4: Advanced resource composition with KRO (planned)
