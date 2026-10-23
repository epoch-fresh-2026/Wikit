ALTER TABLE wikit.blobs DROP CONSTRAINT blobs_state_check;
ALTER TABLE wikit.blobs ADD CHECK(state IN ('pending','ready','deleting','deleted'));
CREATE TABLE wikit.uploads (
 id uuid PRIMARY KEY, owner_id bigint NOT NULL REFERENCES wikit.users(id),
 filename text NOT NULL, mime text NOT NULL, size bigint NOT NULL CHECK(size>0),
 state text NOT NULL CHECK(state IN ('pending','ready','failed')),
 expires_at timestamptz NOT NULL, blob_id uuid REFERENCES wikit.blobs(id),
 attempt_id uuid, attempt_blob_id uuid, lease_until timestamptz,
 CHECK((attempt_id IS NULL AND attempt_blob_id IS NULL AND lease_until IS NULL) OR
       (attempt_id IS NOT NULL AND attempt_blob_id IS NOT NULL AND lease_until IS NOT NULL)),
 CHECK((state='ready')=(blob_id IS NOT NULL))
);
CREATE INDEX ON wikit.uploads(owner_id,expires_at);
CREATE INDEX ON wikit.blobs(created_at) WHERE state='ready';
CREATE INDEX ON wikit.revisions(blob_id) WHERE blob_id IS NOT NULL;
CREATE INDEX ON wikit.proposal_revisions(blob_id) WHERE blob_id IS NOT NULL;
