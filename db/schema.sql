-- MusicKen database (PostgreSQL / Supabase)
-- วิธีใช้: Supabase > SQL Editor > New query > วางไฟล์นี้ทั้งไฟล์ > Run  (รันซ้ำได้ ไม่ลบข้อมูลเดิม)
-- ถ้าเคยรัน schema ของโปรเจกต์อนิเมะไว้ในฐานข้อมูลเดียวกัน ให้ลบตารางเก่าก่อน: db/drop-old-anime.sql

CREATE TABLE IF NOT EXISTS members (
    id            BIGSERIAL PRIMARY KEY,
    email         VARCHAR(255) NOT NULL,
    password_hash VARCHAR(100) NOT NULL,                       -- bcrypt hash ไม่เก็บรหัสผ่านจริง
    display_name  VARCHAR(100) NOT NULL CHECK (char_length(display_name) >= 2),
    role          VARCHAR(10)  NOT NULL DEFAULT 'member' CHECK (role IN ('member', 'admin')),
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_members_email ON members (lower(email));

CREATE TABLE IF NOT EXISTS artists (
    id         BIGSERIAL PRIMARY KEY,
    name       VARCHAR(200) NOT NULL,
    bio        TEXT,
    image_url  TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_artists_name ON artists (lower(name));

CREATE TABLE IF NOT EXISTS songs (
    id           BIGSERIAL PRIMARY KEY,
    title        VARCHAR(255) NOT NULL,
    artist_id    BIGINT NOT NULL REFERENCES artists (id) ON DELETE RESTRICT,   -- ลบศิลปินที่ยังมีเพลงไม่ได้
    album        VARCHAR(255),
    genre        VARCHAR(60),
    year         SMALLINT CHECK (year BETWEEN 1900 AND 2100),
    duration_sec INT CHECK (duration_sec >= 0),
    cover_url    TEXT,
    preview_url  TEXT,                                          -- เสียงตัวอย่าง 30 วินาที (จาก iTunes) ไม่เก็บไฟล์เสียงเอง
    external_id  BIGINT UNIQUE,                                 -- trackId ของ iTunes ใช้กันนำเข้าซ้ำ
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_songs_artist ON songs (artist_id);
CREATE INDEX IF NOT EXISTS idx_songs_genre  ON songs (genre);

CREATE TABLE IF NOT EXISTS playlists (
    id          BIGSERIAL PRIMARY KEY,
    member_id   BIGINT NOT NULL REFERENCES members (id) ON DELETE CASCADE,
    name        VARCHAR(100) NOT NULL CHECK (char_length(name) >= 1),
    description VARCHAR(500) NOT NULL DEFAULT '',
    is_public   BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_playlists_member ON playlists (member_id);

-- เพลงในเพลย์ลิสต์: 1 เพลงอยู่ในลิสต์เดียวกันได้ครั้งเดียว, position = ลำดับ (เรียงน้อยไปมาก)
CREATE TABLE IF NOT EXISTS playlist_songs (
    playlist_id BIGINT NOT NULL REFERENCES playlists (id) ON DELETE CASCADE,
    song_id     BIGINT NOT NULL REFERENCES songs (id) ON DELETE CASCADE,
    position    INT    NOT NULL,
    added_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (playlist_id, song_id)
);
CREATE INDEX IF NOT EXISTS idx_playlist_songs_order ON playlist_songs (playlist_id, position);

CREATE TABLE IF NOT EXISTS playlist_likes (
    member_id   BIGINT NOT NULL REFERENCES members (id) ON DELETE CASCADE,
    playlist_id BIGINT NOT NULL REFERENCES playlists (id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (member_id, playlist_id)
);

-- ประวัติการฟัง (สมาชิกกด play)
CREATE TABLE IF NOT EXISTS plays (
    id        BIGSERIAL PRIMARY KEY,
    member_id BIGINT NOT NULL REFERENCES members (id) ON DELETE CASCADE,
    song_id   BIGINT NOT NULL REFERENCES songs (id) ON DELETE CASCADE,
    played_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_plays_member ON plays (member_id, played_at DESC);
CREATE INDEX IF NOT EXISTS idx_plays_song   ON plays (song_id);

-- สำคัญ: Supabase เปิดตารางใน schema public ให้เรียกผ่าน Data API ด้วย anon key ได้
-- เปิด Row Level Security โดยไม่สร้าง policy = คนนอกอ่าน/เขียนตรง ๆ ไม่ได้ (รวมถึง password_hash)
-- Go เชื่อมด้วยบทบาท postgres ซึ่งข้าม RLS ได้ จึงใช้งานได้ตามปกติ
ALTER TABLE members        ENABLE ROW LEVEL SECURITY;
ALTER TABLE artists        ENABLE ROW LEVEL SECURITY;
ALTER TABLE songs          ENABLE ROW LEVEL SECURITY;
ALTER TABLE playlists      ENABLE ROW LEVEL SECURITY;
ALTER TABLE playlist_songs ENABLE ROW LEVEL SECURITY;
ALTER TABLE playlist_likes ENABLE ROW LEVEL SECURITY;
ALTER TABLE plays          ENABLE ROW LEVEL SECURITY;
