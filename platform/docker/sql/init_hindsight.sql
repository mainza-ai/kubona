-- Kubona Enterprise Hindsight Memory & Distributed Session Locking Schema
-- Reference: pgvector/pgvector:pg16 with HNSW cosine similarity index

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS vector;

-- Multi-Tenant, IFC Scoped Semantic Memory Bank
CREATE TABLE IF NOT EXISTS hindsight_memories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id VARCHAR(64) NOT NULL DEFAULT 'default',
    readership VARCHAR(255) NOT NULL DEFAULT 'public',
    bank VARCHAR(32) NOT NULL,
    content TEXT NOT NULL,
    tokens_estimate INTEGER NOT NULL,
    importance REAL NOT NULL,
    effective_score REAL NOT NULL,
    embedding vector(1536),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_accessed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Fast Approximate Nearest Neighbor Cosine Search (HNSW)
CREATE INDEX IF NOT EXISTS idx_hindsight_vector_hnsw 
ON hindsight_memories USING hnsw (embedding vector_cosine_ops) 
WITH (m = 16, ef_construction = 64);

-- Relational Filtering Indices
CREATE INDEX IF NOT EXISTS idx_hindsight_tenant ON hindsight_memories(tenant_id);
CREATE INDEX IF NOT EXISTS idx_hindsight_bank ON hindsight_memories(bank);
CREATE INDEX IF NOT EXISTS idx_hindsight_score ON hindsight_memories(effective_score);
CREATE INDEX IF NOT EXISTS idx_hindsight_created ON hindsight_memories(created_at);

-- Multi-Node Distributed Session Advisory Locks & Leases
CREATE TABLE IF NOT EXISTS distributed_locks (
    lock_key VARCHAR(128) PRIMARY KEY,
    holder_node_id VARCHAR(128) NOT NULL,
    lease_expires_at TIMESTAMPTZ NOT NULL,
    acquired_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_locks_expiry ON distributed_locks(lease_expires_at);
