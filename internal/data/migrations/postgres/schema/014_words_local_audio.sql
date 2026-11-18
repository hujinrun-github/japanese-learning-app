-- Local TTS files remain usable when no object storage is configured.
ALTER TABLE words ADD COLUMN IF NOT EXISTS local_audio_url TEXT NOT NULL DEFAULT '';
