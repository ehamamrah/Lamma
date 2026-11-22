ALTER TABLE meetings
  ADD COLUMN max_participants int NOT NULL DEFAULT 50
  CHECK (max_participants BETWEEN 2 AND 500);
ALTER TABLE meetings ALTER COLUMN max_participants DROP DEFAULT;
