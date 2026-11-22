-- Meetings no longer have a participant cap: anyone with the link can join
-- unless the host locks the meeting or turns on the waiting room.
ALTER TABLE meetings DROP COLUMN max_participants;
