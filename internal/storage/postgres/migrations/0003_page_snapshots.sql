CREATE TABLE wikit.blobs (
 id uuid PRIMARY KEY, digest text NOT NULL CHECK(length(digest)=64), size bigint NOT NULL CHECK(size>0),
 mime text NOT NULL, storage_key text NOT NULL UNIQUE, state text NOT NULL CHECK(state IN ('pending','ready','deleting')),
 uploader bigint NOT NULL REFERENCES wikit.users(id), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE wikit.pages (
 id bigint PRIMARY KEY REFERENCES wikit.page_policies(page_id),
 type text NOT NULL CHECK(type IN ('document','media')), head_revision_id uuid,
 UNIQUE(id,type)
);
CREATE TABLE wikit.proposals (
 id uuid PRIMARY KEY, page_id bigint NOT NULL REFERENCES wikit.pages(id), author_id bigint NOT NULL REFERENCES wikit.users(id),
 state text NOT NULL CHECK(state IN ('open','closed','merged')), version bigint NOT NULL CHECK(version>0),
 current_revision_id uuid NOT NULL, UNIQUE(id,page_id)
);
CREATE TABLE wikit.revisions (
 id uuid PRIMARY KEY, page_id bigint NOT NULL, type text NOT NULL, parent_id uuid,
 source_proposal_id uuid UNIQUE, snapshot jsonb NOT NULL,
 publisher_id bigint REFERENCES wikit.users(id), author_id bigint REFERENCES wikit.users(id), source_kind text NOT NULL CHECK(source_kind IN ('bootstrap','create','merge')), created_at timestamptz NOT NULL,
 sealed boolean NOT NULL DEFAULT false,
 blob_id uuid GENERATED ALWAYS AS ((snapshot->>'blob_id')::uuid) STORED REFERENCES wikit.blobs(id),
 CHECK((source_kind='bootstrap') = (publisher_id IS NULL)), CHECK((source_kind='merge') = (parent_id IS NOT NULL)), CHECK((source_kind='merge') = (source_proposal_id IS NOT NULL)), CHECK((source_kind='bootstrap') = (author_id IS NULL)), CHECK(source_kind<>'bootstrap' OR page_id=1), CHECK(id IS DISTINCT FROM parent_id), CHECK(snapshot->>'type' IS NOT DISTINCT FROM type),
 UNIQUE(id,page_id), UNIQUE(id,page_id,type), UNIQUE NULLS NOT DISTINCT(page_id,parent_id),
 FOREIGN KEY(page_id,type) REFERENCES wikit.pages(id,type),
 FOREIGN KEY(parent_id,page_id) REFERENCES wikit.revisions(id,page_id),
 FOREIGN KEY(source_proposal_id,page_id) REFERENCES wikit.proposals(id,page_id)
);
CREATE TABLE wikit.proposal_revisions (
 id uuid PRIMARY KEY, proposal_id uuid NOT NULL, page_id bigint NOT NULL, type text NOT NULL,
 base_revision_id uuid, snapshot jsonb NOT NULL, created_at timestamptz NOT NULL,
 sealed boolean NOT NULL DEFAULT false,
 blob_id uuid GENERATED ALWAYS AS ((snapshot->>'blob_id')::uuid) STORED REFERENCES wikit.blobs(id),
 CHECK(snapshot->>'type' IS NOT DISTINCT FROM type),
 UNIQUE(id,proposal_id,page_id),
 FOREIGN KEY(proposal_id,page_id) REFERENCES wikit.proposals(id,page_id),
 FOREIGN KEY(page_id,type) REFERENCES wikit.pages(id,type),
 FOREIGN KEY(base_revision_id,page_id) REFERENCES wikit.revisions(id,page_id)
);
ALTER TABLE wikit.proposals ADD FOREIGN KEY(current_revision_id,id,page_id)
 REFERENCES wikit.proposal_revisions(id,proposal_id,page_id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE wikit.pages ADD FOREIGN KEY(head_revision_id,id)
 REFERENCES wikit.revisions(id,page_id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE wikit.tags (key text PRIMARY KEY);
CREATE TABLE wikit.revision_tags (revision_id uuid REFERENCES wikit.revisions(id), tag_key text REFERENCES wikit.tags(key), PRIMARY KEY(revision_id,tag_key));
CREATE TABLE wikit.proposal_revision_tags (revision_id uuid REFERENCES wikit.proposal_revisions(id), tag_key text REFERENCES wikit.tags(key), PRIMARY KEY(revision_id,tag_key));
CREATE TABLE wikit.revision_media_refs (
 revision_id uuid REFERENCES wikit.revisions(id), target_page_id bigint, target_revision_id uuid,
 target_type text NOT NULL DEFAULT 'media' CHECK(target_type='media'), PRIMARY KEY(revision_id,target_page_id,target_revision_id),
 FOREIGN KEY(target_revision_id,target_page_id,target_type) REFERENCES wikit.revisions(id,page_id,type)
);
CREATE TABLE wikit.proposal_revision_media_refs (
 revision_id uuid REFERENCES wikit.proposal_revisions(id), target_page_id bigint, target_revision_id uuid,
 target_type text NOT NULL DEFAULT 'media' CHECK(target_type='media'), PRIMARY KEY(revision_id,target_page_id,target_revision_id),
 FOREIGN KEY(target_revision_id,target_page_id,target_type) REFERENCES wikit.revisions(id,page_id,type)
);
CREATE FUNCTION wikit.immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'immutable content' USING ERRCODE='23514'; END $$;
CREATE FUNCTION wikit.freeze_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' AND NOT OLD.sealed AND NEW.sealed AND to_jsonb(OLD)-ARRAY['sealed','blob_id']=to_jsonb(NEW)-ARRAY['sealed','blob_id'] THEN RETURN NEW; END IF;
 RAISE EXCEPTION 'immutable snapshot' USING ERRCODE='23514';
END $$;
CREATE FUNCTION wikit.require_sealed() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE frozen boolean;
BEGIN
 EXECUTE format('SELECT sealed FROM %I.%I WHERE id=$1',TG_TABLE_SCHEMA,TG_TABLE_NAME) INTO frozen USING NEW.id;
 IF frozen IS DISTINCT FROM true THEN RAISE EXCEPTION 'unsealed snapshot' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END $$;
CREATE FUNCTION wikit.check_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.type='media' AND NOT EXISTS(SELECT 1 FROM wikit.blobs WHERE id=(NEW.snapshot->>'blob_id')::uuid AND state='ready' FOR SHARE) THEN
  RAISE EXCEPTION 'media requires ready blob' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION wikit.check_parent() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE head uuid;
BEGIN
 SELECT head_revision_id INTO head FROM wikit.pages WHERE id=NEW.page_id FOR UPDATE;
 IF NEW.parent_id IS DISTINCT FROM head THEN RAISE EXCEPTION 'parent is not current head' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION wikit.check_page_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.id<>OLD.id OR NEW.type<>OLD.type THEN RAISE EXCEPTION 'immutable page identity/type' USING ERRCODE='23514'; END IF;
 IF NEW.head_revision_id IS DISTINCT FROM OLD.head_revision_id AND NOT EXISTS(
  SELECT 1 FROM wikit.revisions WHERE id=NEW.head_revision_id AND page_id=NEW.id AND parent_id IS NOT DISTINCT FROM OLD.head_revision_id AND sealed
 ) THEN RAISE EXCEPTION 'head must advance to direct child' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER page_update BEFORE UPDATE ON wikit.pages FOR EACH ROW EXECUTE FUNCTION wikit.check_page_update();
CREATE TRIGGER revision_parent BEFORE INSERT ON wikit.revisions FOR EACH ROW EXECUTE FUNCTION wikit.check_parent();
CREATE FUNCTION wikit.check_association() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE frozen boolean; body jsonb; source_table text;
BEGIN
 source_table:=CASE WHEN TG_TABLE_NAME LIKE 'proposal_%' THEN 'proposal_revisions' ELSE 'revisions' END;
 EXECUTE format('SELECT sealed,snapshot FROM wikit.%I WHERE id=$1',source_table) INTO frozen,body USING NEW.revision_id;
 IF frozen IS DISTINCT FROM false THEN RAISE EXCEPTION 'sealed associations' USING ERRCODE='23514'; END IF;
 IF TG_TABLE_NAME LIKE '%_tags' THEN
  IF NOT (body->'tags' ? NEW.tag_key) THEN RAISE EXCEPTION 'tag not in snapshot' USING ERRCODE='23514'; END IF;
 ELSE
  IF NOT EXISTS(SELECT 1 FROM wikit.pages WHERE id=NEW.target_page_id AND type='media' AND head_revision_id IS NOT NULL) THEN
   RAISE EXCEPTION 'reference requires published media' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END $$;
DO $$ DECLARE name text;
BEGIN
 FOREACH name IN ARRAY ARRAY['revisions','proposal_revisions'] LOOP
  EXECUTE format('CREATE TRIGGER freeze_guard BEFORE UPDATE OR DELETE ON wikit.%I FOR EACH ROW EXECUTE FUNCTION wikit.freeze_snapshot()',name);
  EXECUTE format('CREATE TRIGGER no_truncate BEFORE TRUNCATE ON wikit.%I FOR EACH STATEMENT EXECUTE FUNCTION wikit.immutable()',name);
  EXECUTE format('CREATE TRIGGER validate_snapshot BEFORE INSERT ON wikit.%I FOR EACH ROW EXECUTE FUNCTION wikit.check_snapshot()',name);
  EXECUTE format('CREATE CONSTRAINT TRIGGER seal_check AFTER INSERT OR UPDATE ON wikit.%I DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION wikit.require_sealed()',name);
 END LOOP;
 FOREACH name IN ARRAY ARRAY['revision_tags','proposal_revision_tags','revision_media_refs','proposal_revision_media_refs'] LOOP
  EXECUTE format('CREATE TRIGGER immutable BEFORE UPDATE OR DELETE ON wikit.%I FOR EACH ROW EXECUTE FUNCTION wikit.immutable()',name);
  EXECUTE format('CREATE TRIGGER no_truncate BEFORE TRUNCATE ON wikit.%I FOR EACH STATEMENT EXECUTE FUNCTION wikit.immutable()',name);
  EXECUTE format('CREATE TRIGGER validate_association BEFORE INSERT ON wikit.%I FOR EACH ROW EXECUTE FUNCTION wikit.check_association()',name);
 END LOOP;
END $$;
CREATE TRIGGER immutable BEFORE UPDATE OR DELETE ON wikit.tags FOR EACH ROW EXECUTE FUNCTION wikit.immutable();
CREATE INDEX ON wikit.revisions(page_id,created_at,id);
CREATE INDEX ON wikit.proposals(page_id,state,id);
CREATE INDEX ON wikit.proposal_revisions(proposal_id,created_at,id);

CREATE TRIGGER page_delete BEFORE DELETE ON wikit.pages FOR EACH ROW EXECUTE FUNCTION wikit.immutable();
CREATE TRIGGER page_truncate BEFORE TRUNCATE ON wikit.pages FOR EACH STATEMENT EXECUTE FUNCTION wikit.immutable();
