ALTER TABLE wikit.outbox_events ALTER COLUMN revision_id DROP NOT NULL;
ALTER TABLE wikit.outbox_events DROP CONSTRAINT outbox_events_kind_check;
ALTER TABLE wikit.outbox_events ADD CONSTRAINT outbox_events_kind_check CHECK(kind IN ('page.published','page.policy','page.repair'));
ALTER TABLE wikit.outbox_events ADD COLUMN sequence bigserial UNIQUE;
ALTER TABLE wikit.outbox_events ADD COLUMN task_uid bigint CHECK(task_uid>=0);
ALTER TABLE wikit.outbox_events ADD COLUMN target_index text;
ALTER TABLE wikit.outbox_events ADD COLUMN last_error text CHECK(last_error IN ('submit_unavailable','task_unavailable'));
CREATE INDEX outbox_page_order ON wikit.outbox_events(page_id,sequence) WHERE completed_at IS NULL;
CREATE UNIQUE INDEX outbox_pending_repair ON wikit.outbox_events(page_id) WHERE kind='page.repair' AND completed_at IS NULL;
CREATE TABLE wikit.search_state(singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),active_index text NOT NULL);
INSERT INTO wikit.search_state VALUES(true,'wikit_pages');
CREATE FUNCTION wikit.enqueue_policy_projection() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.policy_version<>OLD.policy_version OR NEW.visibility<>OLD.visibility OR (NEW.status<>OLD.status AND (NEW.status='hidden' OR OLD.status='hidden')))
 AND EXISTS(SELECT 1 FROM wikit.pages WHERE id=NEW.page_id AND head_revision_id IS NOT NULL) THEN
  INSERT INTO wikit.outbox_events(id,page_id,revision_id,kind,created_at) VALUES(gen_random_uuid(),NEW.page_id,NULL,'page.policy',now());
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER policy_projection AFTER UPDATE ON wikit.page_policies FOR EACH ROW EXECUTE FUNCTION wikit.enqueue_policy_projection();
