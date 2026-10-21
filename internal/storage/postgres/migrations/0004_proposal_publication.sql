CREATE TABLE wikit.outbox_events (
 id uuid PRIMARY KEY, page_id bigint NOT NULL, revision_id uuid NOT NULL UNIQUE,
 kind text NOT NULL CHECK(kind='page.published'), created_at timestamptz NOT NULL,
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts>=0), next_attempt timestamptz NOT NULL DEFAULT now(),
 lease_until timestamptz, completed_at timestamptz,
 FOREIGN KEY(revision_id,page_id) REFERENCES wikit.revisions(id,page_id)
);
CREATE INDEX ON wikit.outbox_events(next_attempt) WHERE completed_at IS NULL;
