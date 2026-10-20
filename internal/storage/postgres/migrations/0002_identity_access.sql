CREATE TABLE wikit.users (
    id uuid PRIMARY KEY, username text NOT NULL,
    username_key text NOT NULL UNIQUE CHECK (username_key = lower(username)),
    display_name text NOT NULL, password_hash text NOT NULL,
    role text NOT NULL CHECK (role IN ('member','reviewer','admin')),
    status text NOT NULL CHECK (status IN ('active','disabled')),
    auth_version bigint NOT NULL CHECK (auth_version > 0)
);
CREATE TABLE wikit.user_tokens (
    user_id uuid PRIMARY KEY REFERENCES wikit.users(id), digest text UNIQUE,
    generation bigint NOT NULL CHECK (generation > 0), created_at timestamptz NOT NULL
);
CREATE TABLE wikit.sessions (
    digest text PRIMARY KEY, user_id uuid NOT NULL REFERENCES wikit.users(id),
    csrf_digest text NOT NULL, auth_version bigint NOT NULL, expires_at timestamptz NOT NULL
);
CREATE INDEX ON wikit.sessions(user_id);
CREATE TABLE wikit.groups (id uuid PRIMARY KEY, name text NOT NULL, version bigint NOT NULL);
CREATE TABLE wikit.group_members (
    group_id uuid REFERENCES wikit.groups(id), user_id uuid REFERENCES wikit.users(id),
    PRIMARY KEY(group_id,user_id)
);
CREATE TABLE wikit.page_policies (
    page_id uuid PRIMARY KEY, visibility text NOT NULL CHECK (visibility IN ('public','restricted')),
    status text NOT NULL CHECK (status IN ('proposed','published','hidden')),
    policy_version bigint NOT NULL CHECK (policy_version > 0)
);
CREATE TABLE wikit.page_grants (
    page_id uuid REFERENCES wikit.page_policies(page_id),
    subject_type text NOT NULL CHECK (subject_type IN ('user','group')), subject_id uuid NOT NULL,
    capability text NOT NULL CHECK (capability IN ('read','propose','discuss','review','manage')),
    PRIMARY KEY(page_id,subject_type,subject_id,capability)
);
CREATE TABLE wikit.audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), actor_id uuid REFERENCES wikit.users(id),
    method text NOT NULL, credential_generation bigint NOT NULL, action text NOT NULL,
    target_id uuid, request_id text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE wikit.auth_limits (key text PRIMARY KEY, window_start timestamptz NOT NULL, attempts integer NOT NULL);
CREATE TABLE wikit.idempotency_records (
    actor_id uuid REFERENCES wikit.users(id), operation text NOT NULL, key text NOT NULL,
    request_hash text NOT NULL, result jsonb NOT NULL, expires_at timestamptz NOT NULL,
    PRIMARY KEY(actor_id,operation,key)
);
CREATE TABLE wikit.list_cursors (
    digest text PRIMARY KEY, actor_id uuid REFERENCES wikit.users(id), kind text NOT NULL,
    page_size integer NOT NULL, position integer NOT NULL, expires_at timestamptz NOT NULL
);
