CREATE TABLE wikit.page_issues (
 id uuid PRIMARY KEY, page_id bigint NOT NULL REFERENCES wikit.pages(id),
 author_id bigint NOT NULL REFERENCES wikit.users(id),
 title text NOT NULL CHECK(char_length(title) BETWEEN 1 AND 300),
 body text NOT NULL CHECK(char_length(body)<=65536),
 state text NOT NULL CHECK(state IN ('open','closed')), hidden boolean NOT NULL DEFAULT false,
 version bigint NOT NULL CHECK(version>0), created_at timestamptz NOT NULL,
 UNIQUE(id,page_id)
);
CREATE TABLE wikit.comments (
 id uuid PRIMARY KEY, issue_id uuid NOT NULL REFERENCES wikit.page_issues(id),
 author_id bigint NOT NULL REFERENCES wikit.users(id), body text NOT NULL CHECK(char_length(body) BETWEEN 1 AND 65536),
 hidden boolean NOT NULL DEFAULT false, version bigint NOT NULL CHECK(version>0), created_at timestamptz NOT NULL
);
CREATE TABLE wikit.issue_proposals (
 issue_id uuid NOT NULL, proposal_id uuid NOT NULL, page_id bigint NOT NULL,
 PRIMARY KEY(issue_id,proposal_id),
 FOREIGN KEY(issue_id,page_id) REFERENCES wikit.page_issues(id,page_id),
 FOREIGN KEY(proposal_id,page_id) REFERENCES wikit.proposals(id,page_id)
);
CREATE TABLE wikit.moderation_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),page_id bigint NOT NULL REFERENCES wikit.pages(id),target_id text NOT NULL,
 kind text NOT NULL CHECK(kind IN ('page','issue','comment')),actor_id bigint NOT NULL REFERENCES wikit.users(id),
 hidden boolean NOT NULL,reason text NOT NULL CHECK(char_length(reason) BETWEEN 1 AND 2000),
 version bigint NOT NULL CHECK(version>0),request_id text NOT NULL,created_at timestamptz NOT NULL DEFAULT now()
);
CREATE FUNCTION wikit.freeze_discussion() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='page_issues' THEN
  IF (to_jsonb(OLD)-ARRAY['state','hidden','version']) IS DISTINCT FROM (to_jsonb(NEW)-ARRAY['state','hidden','version']) THEN
   RAISE EXCEPTION 'immutable discussion content' USING ERRCODE='23514';
  END IF;
 ELSE
  IF (to_jsonb(OLD)-ARRAY['hidden','version']) IS DISTINCT FROM (to_jsonb(NEW)-ARRAY['hidden','version']) THEN
   RAISE EXCEPTION 'immutable discussion content' USING ERRCODE='23514';
  END IF;
 END IF;
 IF NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'invalid discussion version' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER issue_update BEFORE UPDATE ON wikit.page_issues FOR EACH ROW EXECUTE FUNCTION wikit.freeze_discussion();
CREATE TRIGGER comment_update BEFORE UPDATE ON wikit.comments FOR EACH ROW EXECUTE FUNCTION wikit.freeze_discussion();
CREATE TRIGGER issue_delete BEFORE DELETE ON wikit.page_issues FOR EACH ROW EXECUTE FUNCTION wikit.immutable();
CREATE TRIGGER comment_delete BEFORE DELETE ON wikit.comments FOR EACH ROW EXECUTE FUNCTION wikit.immutable();
CREATE TRIGGER issue_truncate BEFORE TRUNCATE ON wikit.page_issues EXECUTE FUNCTION wikit.immutable();
CREATE TRIGGER comment_truncate BEFORE TRUNCATE ON wikit.comments EXECUTE FUNCTION wikit.immutable();
CREATE TRIGGER link_immutable BEFORE UPDATE OR DELETE ON wikit.issue_proposals FOR EACH ROW EXECUTE FUNCTION wikit.immutable();
CREATE TRIGGER link_truncate BEFORE TRUNCATE ON wikit.issue_proposals EXECUTE FUNCTION wikit.immutable();
CREATE TRIGGER moderation_immutable BEFORE UPDATE OR DELETE ON wikit.moderation_events FOR EACH ROW EXECUTE FUNCTION wikit.immutable();
CREATE TRIGGER moderation_truncate BEFORE TRUNCATE ON wikit.moderation_events EXECUTE FUNCTION wikit.immutable();
CREATE INDEX issues_page ON wikit.page_issues(page_id,state,created_at,id);
CREATE INDEX comments_issue ON wikit.comments(issue_id,created_at,id);
CREATE INDEX moderation_page ON wikit.moderation_events(page_id,created_at,id);
CREATE FUNCTION wikit.check_moderation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE actual_hidden boolean; actual_version bigint;
BEGIN
 IF NEW.kind='page' THEN
  SELECT status='hidden',policy_version INTO actual_hidden,actual_version FROM wikit.page_policies WHERE page_id=NEW.page_id AND page_id::text=NEW.target_id;
 ELSIF NEW.kind='issue' THEN
  SELECT hidden,version INTO actual_hidden,actual_version FROM wikit.page_issues WHERE id::text=NEW.target_id AND page_id=NEW.page_id;
 ELSE
  SELECT c.hidden,c.version INTO actual_hidden,actual_version FROM wikit.comments c JOIN wikit.page_issues i ON i.id=c.issue_id WHERE c.id::text=NEW.target_id AND i.page_id=NEW.page_id;
 END IF;
 IF actual_hidden IS DISTINCT FROM NEW.hidden OR actual_version IS DISTINCT FROM NEW.version THEN
  RAISE EXCEPTION 'moderation target mismatch' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER moderation_target BEFORE INSERT ON wikit.moderation_events FOR EACH ROW EXECUTE FUNCTION wikit.check_moderation();
