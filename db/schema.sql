-- AniWave database (PostgreSQL / Supabase)
-- วิธีใช้: Supabase > SQL Editor > New query > วางไฟล์นี้ทั้งไฟล์ > Run  (รันซ้ำได้ ไม่ลบข้อมูลเดิม)

CREATE TABLE IF NOT EXISTS members (
    id            BIGSERIAL PRIMARY KEY,
    email         VARCHAR(255) NOT NULL,
    password_hash VARCHAR(100) NOT NULL,                       -- bcrypt hash ไม่เก็บรหัสผ่านจริง
    display_name  VARCHAR(100) NOT NULL CHECK (char_length(display_name) >= 2),
    role          VARCHAR(10)  NOT NULL DEFAULT 'member' CHECK (role IN ('member', 'admin')),
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);
-- อีเมลห้ามซ้ำ โดยไม่สนตัวพิมพ์เล็กใหญ่ (ตอน INSERT ซ้ำ Go จะจับ error 23505 แล้วตอบ 409)
CREATE UNIQUE INDEX IF NOT EXISTS uq_members_email ON members (lower(email));

CREATE TABLE IF NOT EXISTS anime (
    id         BIGSERIAL PRIMARY KEY,
    title      VARCHAR(255) NOT NULL,
    title_th   VARCHAR(255),
    year       SMALLINT CHECK (year BETWEEN 1900 AND 2100),
    genre      VARCHAR(50),
    rating     NUMERIC(3, 1) CHECK (rating BETWEEN 0 AND 10),   -- คะแนนตั้งต้นของเรื่อง (ไม่ใช่คะแนนโหวตของสมาชิก)
    episodes   INT CHECK (episodes >= 0),
    status     VARCHAR(10) NOT NULL DEFAULT 'finished' CHECK (status IN ('airing', 'finished', 'upcoming')),
    poster     TEXT,
    synopsis   TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- หมายเหตุ: ข้อมูลอนิเมะที่คนทั่วไปเห็นมาจาก TENRAI ที่ FE ดึงเอง reviews / votes / wishlist_items จึงเก็บแค่ anime_id (= mal_id ของ TENRAI)
-- ไม่อ้างอิงตาราง anime (ตาราง anime ใช้เป็นรายการของทีมที่ admin จัดการ) ถ้าฐานข้อมูลถูกสร้างไปแล้วให้รัน db/migrate-tenrai-ids.sql

-- รีวิว: สมาชิก 1 คนรีวิวเรื่องเดียวกันได้หลายครั้ง (ตรงกับ docs/api-contract.md ที่ไม่มี endpoint แก้รีวิว)
CREATE TABLE IF NOT EXISTS reviews (
    id         BIGSERIAL PRIMARY KEY,
    member_id  BIGINT NOT NULL REFERENCES members (id) ON DELETE CASCADE,
    anime_id   BIGINT NOT NULL,                                 -- mal_id ของ TENRAI (ไม่ผูก FK กับตาราง anime)
    text       TEXT   NOT NULL CHECK (char_length(text) >= 10),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_reviews_anime ON reviews (anime_id, created_at DESC);

-- คะแนน 1-10: 1 คน 1 เรื่อง 1 คะแนน (โหวตใหม่ = แก้คะแนนเดิม)
CREATE TABLE IF NOT EXISTS votes (
    member_id  BIGINT   NOT NULL REFERENCES members (id) ON DELETE CASCADE,
    anime_id   BIGINT   NOT NULL,                               -- mal_id ของ TENRAI (ไม่ผูก FK กับตาราง anime)
    score      SMALLINT NOT NULL CHECK (score BETWEEN 1 AND 10),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (member_id, anime_id)
);
CREATE INDEX IF NOT EXISTS idx_votes_anime ON votes (anime_id);

CREATE TABLE IF NOT EXISTS wishlist_items (
    member_id BIGINT NOT NULL REFERENCES members (id) ON DELETE CASCADE,
    anime_id  BIGINT NOT NULL,                                  -- mal_id ของ TENRAI (ไม่ผูก FK กับตาราง anime)
    added_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (member_id, anime_id)
);

-- สำคัญ: Supabase เปิดตารางใน schema public ให้เรียกผ่าน Data API ด้วย anon key ได้
-- เปิด Row Level Security โดยไม่สร้าง policy = คนนอกอ่าน/เขียนตรง ๆ ไม่ได้ (รวมถึง password_hash)
-- Go เชื่อมด้วยบทบาท postgres ซึ่งข้าม RLS ได้ จึงใช้งานได้ตามปกติ
ALTER TABLE members        ENABLE ROW LEVEL SECURITY;
ALTER TABLE anime          ENABLE ROW LEVEL SECURITY;
ALTER TABLE reviews        ENABLE ROW LEVEL SECURITY;
ALTER TABLE votes          ENABLE ROW LEVEL SECURITY;
ALTER TABLE wishlist_items ENABLE ROW LEVEL SECURITY;
