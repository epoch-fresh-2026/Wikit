ALTER TABLE wikit.list_cursors
 ADD COLUMN page_anchor bigint CHECK (page_anchor IS NULL OR page_anchor > 0);
