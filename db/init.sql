CREATE TABLE IF NOT EXISTS analyses (
  id TEXT PRIMARY KEY,
  input_type TEXT NOT NULL,
  source_text TEXT NOT NULL,
  image_path TEXT,
  structured_messages JSONB NOT NULL DEFAULT '[]'::jsonb,
  metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
  result JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS analyses_created_at_idx ON analyses (created_at DESC);

