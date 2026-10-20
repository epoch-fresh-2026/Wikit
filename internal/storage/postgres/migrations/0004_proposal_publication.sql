CREATE TABLE wikit.reviews (
 sequence bigint GENERATED ALWAYS AS IDENTITY UNIQUE,
 id uuid PRIMARY KEY, proposal_id uuid NOT NULL, page_id uuid NOT NULL,
 proposal_revision_id uuid NOT NULL, base_revision_id uuid,
 round bigint NOT NULL CHECK(round>0), actor_id uuid NOT NULL REFERENCES wikit.users(id),
 decision text NOT NULL CHECK(decision IN ('approve','request_changes','comment')),
 body text NOT NULL CHECK(char_length(body)<=65536), created_at timestamptz NOT NULL,
 FOREIGN KEY(proposal_id,page_id) REFERENCES wikit.proposals(id,page_id),
 FOREIGN KEY(proposal_revision_id,proposal_id,page_id) REFERENCES wikit.proposal_revisions(id,proposal_id,page_id),
 FOREIGN KEY(base_revision_id,page_id) REFERENCES wikit.revisions(id,page_id)
);
CREATE FUNCTION wikit.check_review() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM wikit.proposals p JOIN wikit.proposal_revisions r ON r.id=p.current_revision_id
  JOIN wikit.pages page ON page.id=p.page_id
  WHERE p.id=NEW.proposal_id AND p.state='open' AND p.current_revision_id=NEW.proposal_revision_id
  AND p.review_round=NEW.round AND r.sealed
  AND r.base_revision_id IS NOT DISTINCT FROM NEW.base_revision_id
  AND page.head_revision_id IS NOT DISTINCT FROM NEW.base_revision_id) THEN
  RAISE EXCEPTION 'review must bind current candidate, head and round' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER review_binding BEFORE INSERT ON wikit.reviews FOR EACH ROW EXECUTE FUNCTION wikit.check_review();
CREATE TRIGGER immutable BEFORE UPDATE OR DELETE ON wikit.reviews FOR EACH ROW EXECUTE FUNCTION wikit.immutable();
CREATE TRIGGER no_truncate BEFORE TRUNCATE ON wikit.reviews FOR EACH STATEMENT EXECUTE FUNCTION wikit.immutable();
CREATE INDEX ON wikit.reviews(proposal_id,sequence);
CREATE TABLE wikit.outbox_events (
 id uuid PRIMARY KEY, page_id uuid NOT NULL, revision_id uuid NOT NULL UNIQUE,
 kind text NOT NULL CHECK(kind='page.published'), created_at timestamptz NOT NULL,
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts>=0), next_attempt timestamptz NOT NULL DEFAULT now(),
 lease_until timestamptz, completed_at timestamptz,
 FOREIGN KEY(revision_id,page_id) REFERENCES wikit.revisions(id,page_id)
);
CREATE INDEX ON wikit.outbox_events(next_attempt) WHERE completed_at IS NULL;
