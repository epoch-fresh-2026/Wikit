-- New-deployment bootstrap. Migration history prevents a second publication.
DO $$ DECLARE revision uuid := gen_random_uuid(); BEGIN
 INSERT INTO wikit.page_policies(page_id,visibility,status,policy_version) VALUES(1,'public','published',1);
 INSERT INTO wikit.pages(id,type) VALUES(1,'document');
 INSERT INTO wikit.revisions(id,page_id,type,snapshot,source_kind,created_at)
 VALUES(revision,1,'document','{"type":"document","title":"首页","tags":[],"markdown":""}','bootstrap',now());
 UPDATE wikit.revisions SET sealed=true WHERE id=revision;
 UPDATE wikit.pages SET head_revision_id=revision WHERE id=1;
 INSERT INTO wikit.outbox_events(id,page_id,revision_id,kind,created_at) VALUES(revision,1,revision,'page.published',now());
 INSERT INTO wikit.audit_events(method,credential_generation,action,target_id,request_id)
 VALUES('system',0,'page.bootstrap','1','migration:0008');
END $$;

CREATE FUNCTION wikit.protect_home_policy() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.page_id=1 THEN RAISE EXCEPTION 'home policy cannot be deleted' USING ERRCODE='23514'; END IF;
 RETURN OLD;
END $$;
CREATE TRIGGER home_policy_delete BEFORE DELETE ON wikit.page_policies FOR EACH ROW EXECUTE FUNCTION wikit.protect_home_policy();
CREATE TRIGGER policy_truncate BEFORE TRUNCATE ON wikit.page_policies FOR EACH STATEMENT EXECUTE FUNCTION wikit.immutable();

CREATE FUNCTION wikit.protect_home_publication() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.page_id=1 AND NOT EXISTS(SELECT 1 FROM wikit.users WHERE id=NEW.publisher_id AND role='admin' AND status='active') THEN
  RAISE EXCEPTION 'home publication requires an active administrator' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER home_publication BEFORE INSERT ON wikit.revisions FOR EACH ROW EXECUTE FUNCTION wikit.protect_home_publication();
