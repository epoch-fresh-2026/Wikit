CREATE SEQUENCE wikit.account_ids AS bigint MINVALUE 1 NO CYCLE;
CREATE TABLE wikit.users (
    id bigint PRIMARY KEY DEFAULT nextval('wikit.account_ids') CHECK(id>0), username text NOT NULL,
    username_key text NOT NULL UNIQUE CHECK (username_key = lower(username)),
    display_name text NOT NULL, password_hash text NOT NULL,
    role text NOT NULL CHECK (role IN ('member','moderator','admin')),
    status text NOT NULL CHECK (status IN ('active','disabled')),
    auth_version bigint NOT NULL CHECK (auth_version > 0), create_page boolean NOT NULL DEFAULT false
);
ALTER SEQUENCE wikit.account_ids OWNED BY wikit.users.id;
CREATE TABLE wikit.user_tokens (
    user_id bigint PRIMARY KEY REFERENCES wikit.users(id), digest text UNIQUE,
    generation bigint NOT NULL CHECK (generation > 0), created_at timestamptz NOT NULL
);
CREATE TABLE wikit.sessions (
    digest text PRIMARY KEY, user_id bigint NOT NULL REFERENCES wikit.users(id),
    csrf_digest text NOT NULL, auth_version bigint NOT NULL, expires_at timestamptz NOT NULL
);
CREATE INDEX ON wikit.sessions(user_id);
CREATE TABLE wikit.groups (id uuid PRIMARY KEY, name text NOT NULL, version bigint NOT NULL, create_page boolean NOT NULL DEFAULT false);
CREATE TABLE wikit.group_members (
    group_id uuid REFERENCES wikit.groups(id), user_id bigint REFERENCES wikit.users(id),
    PRIMARY KEY(group_id,user_id)
);
CREATE SEQUENCE wikit.page_ids AS bigint START 2;
CREATE TABLE wikit.page_policies (
    page_id bigint PRIMARY KEY CHECK(page_id>0), visibility text NOT NULL CHECK (visibility IN ('public','restricted')),
    status text NOT NULL CHECK (status IN ('published','hidden')),
    policy_version bigint NOT NULL CHECK (policy_version > 0), CHECK(page_id<>1 OR (visibility='public' AND status='published'))
);
CREATE TABLE wikit.page_grants (
    page_id bigint REFERENCES wikit.page_policies(page_id),
    subject_type text NOT NULL CHECK (subject_type IN ('user','group')), subject_id text NOT NULL,
    capability text NOT NULL CHECK (capability IN ('read','propose','discuss','merge','manage')),
    PRIMARY KEY(page_id,subject_type,subject_id,capability),
    CHECK(CASE subject_type
      WHEN 'user' THEN CASE WHEN subject_id ~ '^[1-9][0-9]{0,18}$' THEN subject_id::numeric<=9223372036854775807 ELSE false END
      WHEN 'group' THEN subject_id ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
      ELSE false END)
);
CREATE TABLE wikit.audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), actor_id bigint REFERENCES wikit.users(id),
    method text NOT NULL, credential_generation bigint NOT NULL, action text NOT NULL,
    target_id text, request_id text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE wikit.auth_limits (key text PRIMARY KEY, window_start timestamptz NOT NULL, attempts integer NOT NULL);
CREATE TABLE wikit.idempotency_records (
    actor_id bigint REFERENCES wikit.users(id), operation text NOT NULL, key text NOT NULL,
    request_hash text NOT NULL, result jsonb NOT NULL, expires_at timestamptz NOT NULL,
    PRIMARY KEY(actor_id,operation,key)
);
CREATE TABLE wikit.list_cursors (
    digest text PRIMARY KEY, actor_id bigint REFERENCES wikit.users(id), kind text NOT NULL,
    page_size integer NOT NULL, position integer NOT NULL, expires_at timestamptz NOT NULL
);
