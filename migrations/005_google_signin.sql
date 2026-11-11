-- Sign-in with Google: a user can be known by a Google email instead of a password.
ALTER TABLE users ADD COLUMN email TEXT UNIQUE;
ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;
ALTER TABLE users ADD CONSTRAINT users_have_a_way_in CHECK (password_hash IS NOT NULL OR email IS NOT NULL);
