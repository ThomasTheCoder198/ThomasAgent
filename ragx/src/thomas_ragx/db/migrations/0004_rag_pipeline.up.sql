-- RAG pipeline (M1.3). Blocks, the document tree and table JSON live here; vectors live in Qdrant.
-- collection_name is the KB's Qdrant collection pointer (ruling A20): one collection per embedding model + dims.
ALTER TABLE knowledge_bases ADD COLUMN collection_name text;
UPDATE knowledge_bases
SET collection_name = 'kb_' || replace(embedding_model_id::text, '-', '') || '_' || embedding_dims;
ALTER TABLE knowledge_bases ALTER COLUMN collection_name SET NOT NULL;

ALTER TABLE kb_files
    ADD COLUMN parse_provider     text        NOT NULL DEFAULT '',
    ADD COLUMN parse_quality      real,
    ADD COLUMN page_count         integer,
    ADD COLUMN chunk_count        integer     NOT NULL DEFAULT 0,
    ADD COLUMN ingest_lease_until timestamptz,
    ADD COLUMN ingested_at        timestamptz;

CREATE TABLE doc_nodes (
    id         uuid        PRIMARY KEY,
    org_id     text        NOT NULL,
    kb_id      uuid        NOT NULL,
    file_id    uuid        NOT NULL REFERENCES kb_files (id) ON DELETE CASCADE,
    parent_id  uuid        REFERENCES doc_nodes (id) ON DELETE CASCADE,
    position   integer     NOT NULL,
    depth      integer     NOT NULL,
    title      text        NOT NULL,
    summary    text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_doc_nodes_file_id_position ON doc_nodes (file_id, position);

CREATE TABLE doc_blocks (
    file_id    uuid    NOT NULL REFERENCES kb_files (id) ON DELETE CASCADE,
    block_id   text    NOT NULL,
    org_id     text    NOT NULL,
    kb_id      uuid    NOT NULL,
    node_id    uuid    NOT NULL REFERENCES doc_nodes (id) ON DELETE CASCADE,
    position   integer NOT NULL,
    block_type text    NOT NULL,
    text       text    NOT NULL,
    locator    jsonb   NOT NULL,
    PRIMARY KEY (file_id, block_id)
);
CREATE INDEX idx_doc_blocks_node_id_position ON doc_blocks (node_id, position);

CREATE TABLE kb_tables (
    id         uuid        PRIMARY KEY,
    org_id     text        NOT NULL,
    kb_id      uuid        NOT NULL,
    file_id    uuid        NOT NULL REFERENCES kb_files (id) ON DELETE CASCADE,
    node_id    uuid        NOT NULL REFERENCES doc_nodes (id) ON DELETE CASCADE,
    block_id   text        NOT NULL,
    header     jsonb       NOT NULL,
    rows       jsonb       NOT NULL,
    locator    jsonb       NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_kb_tables_file_id ON kb_tables (file_id);

-- Shared across orgs on purpose: equal text under one model has one vector (main spec §4.2 step 6).
CREATE TABLE embedding_cache (
    model_id       uuid        NOT NULL REFERENCES models (id) ON DELETE CASCADE,
    content_sha256 text        NOT NULL,
    dims           integer     NOT NULL,
    vector         bytea       NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (model_id, content_sha256)
);
