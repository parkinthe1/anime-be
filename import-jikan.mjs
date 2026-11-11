// ดึงอนิเมะจาก Jikan API แล้วสร้างไฟล์ SQL ไปวางใน Supabase SQL Editor
// ใช้: node import-jikan.mjs [จำนวนเรื่อง=100]   -> ได้ไฟล์ jikan_import.sql
// ต้องใช้ Node 18 ขึ้นไป (มี fetch ในตัว) ไม่ต้องติดตั้งแพ็กเกจเพิ่ม
//
// ผลของ SQL ที่ได้ (รันซ้ำได้ ไม่สร้างเรื่องซ้ำ):
//   1) เพิ่มเรื่องใหม่ที่ยังไม่มีในตาราง anime (เทียบจาก title แบบไม่สนตัวพิมพ์เล็กใหญ่)
//   2) เติมรูป poster ให้เรื่องเดิมที่ poster ยังเป็น NULL (เช่น 12 เรื่องจาก seed.sql)
// ไม่แตะ id เดิม รีวิว/โหวต/wishlist ที่ผูกไว้จึงไม่หาย

import { writeFileSync } from 'node:fs';

const TENRAI = 'https://api.tenrai.org/v1';
const TARGET = Math.max(1, parseInt(process.argv[2] || '100', 10));
const OUT = 'tenrai_import.sql';

// ชื่อ (title) ของ 12 เรื่องใน db/seed.sql ใช้ค้นหารูปให้ ถ้าไม่อยู่ใน top ที่ดึงมา
const SEED_TITLES = [
  'Fullmetal Alchemist: Brotherhood', 'Sousou no Frieren', 'Spy x Family',
  'Kimetsu no Yaiba', 'Jujutsu Kaisen', 'Shingeki no Kyojin', 'One Piece',
  'Kimi no Na wa.', 'Cowboy Bebop', 'Steins;Gate', 'Kaguya-sama: Love is War',
  'Violet Evergarden',
];

const STATUS = { 'Currently Airing': 'airing', 'Finished Airing': 'finished', 'Not yet aired': 'upcoming' };
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// เรียก Jikan พร้อมเว้นจังหวะ (จำกัด ~3 req/วินาที, 60 req/นาที) และลองใหม่ถ้าโดน 429
async function tenrai(path, tries = 5) {
  for (let i = 0; i < tries; i++) {
    await sleep(1100);
    const res = await fetch(TENRAI + path);
    if (res.ok) return res.json();
    if (res.status === 429 || res.status >= 500) {
      console.log(`  TENRAI ตอบ ${res.status} รอแล้วลองใหม่...`);
      await sleep(3000 * (i + 1));
      continue;
    }
    throw new Error(`TENRAI ตอบ ${res.status} ที่ ${path}`);
  }
  throw new Error(`TENRAI ไม่ตอบหลังลอง ${tries} ครั้ง: ${path}`);
}

function toRow(j) {
  const synopsis = (j.synopsis ?? '')
    .replace(/\[Written by MAL Rewrite\]/gi, '')
    .replace(/\(Source:[^)]*\)/gi, '')
    .replace(/\s+\n/g, '\n')
    .trim();
  return {
    title: j.title,                                      // ชื่อ romaji ให้เหมือนรูปแบบใน seed.sql
    year: j.year ?? j.aired?.prop?.from?.year ?? null,
    genre: j.genres?.[0]?.name ?? null,
    rating: j.score != null ? Math.round(j.score * 10) / 10 : null,
    episodes: j.episodes ?? null,
    status: STATUS[j.status] ?? 'finished',
    poster: j.images?.jpg?.large_image_url ?? j.images?.jpg?.image_url ?? null,
    synopsis,
  };
}

// แปลงค่าเป็น SQL literal (ครอบ ' เป็น '' และตัดอักขระ NUL ที่ Postgres ไม่รับ)
const q = (s) => (s == null ? 'NULL' : `'${String(s).replace(/\0/g, '').replace(/'/g, "''")}'`);
const n = (x) => (x == null ? 'NULL' : String(Number(x)));

async function main() {
  const byTitle = new Map();   // lower(title) -> row (กันซ้ำ)
  const add = (j) => {
    const r = toRow(j);
    if (r.title && !byTitle.has(r.title.toLowerCase())) byTitle.set(r.title.toLowerCase(), r);
  };

  // 1) ดึง top anime ทีละหน้า (หน้าละ 25 เรื่อง)
  for (let page = 1; byTitle.size < TARGET; page++) {
    console.log(`ดึง top anime หน้า ${page} (มี ${byTitle.size}/${TARGET})`);
    const data = await tenrai(`/top/anime?page=${page}&limit=25&sfw=true`);
    for (const j of data.data) { if (byTitle.size < TARGET) add(j); }
    if (!data.pagination?.has_next_page) break;
  }

  // 2) 12 เรื่องจาก seed ที่ไม่อยู่ใน top ให้ค้นหาตามชื่อ (รับเฉพาะที่ชื่อตรงเป๊ะ กันได้รูปผิดเรื่อง)
  for (const t of SEED_TITLES) {
    if (byTitle.has(t.toLowerCase())) continue;
    console.log(`ค้นหา "${t}"`);
    const data = await tenrai(`/anime?q=${encodeURIComponent(t)}&limit=5&sfw=true`);
    const hit = data.data.find((j) => j.title?.toLowerCase() === t.toLowerCase());
    if (hit) add(hit); else console.log(`  ไม่พบชื่อตรงกับ "${t}" ข้ามไป`);
  }

  const rows = [...byTitle.values()];
  const values = rows
    .map((r) => `  (${q(r.title)}, ${n(r.year)}, ${q(r.genre)}, ${n(r.rating)}, ${n(r.episodes)}, ${q(r.status)}, ${q(r.poster)}, ${q(r.synopsis)})`)
    .join(',\n');

  const sql = `-- สร้างโดย import-jikan.mjs เมื่อ ${new Date().toISOString()} (${rows.length} เรื่อง)
-- วางทั้งไฟล์ใน Supabase > SQL Editor แล้ว Run รันซ้ำได้ ไม่เพิ่มเรื่องซ้ำ

WITH v (title, year, genre, rating, episodes, status, poster, synopsis) AS (
VALUES
${values}
),
upd AS (
  -- เติมรูปให้เรื่องเดิมที่ยังไม่มีรูป
  UPDATE anime a
     SET poster = v.poster, updated_at = now()
    FROM v
   WHERE lower(a.title) = lower(v.title) AND a.poster IS NULL AND v.poster IS NOT NULL
  RETURNING a.id
)
-- เพิ่มเรื่องใหม่ (id ให้ฐานข้อมูลสร้างเอง)
INSERT INTO anime (title, year, genre, rating, episodes, status, poster, synopsis)
SELECT v.title, v.year::smallint, v.genre, v.rating::numeric(3,1), v.episodes::int, v.status, v.poster, coalesce(v.synopsis, '')
  FROM v
 WHERE NOT EXISTS (SELECT 1 FROM anime a WHERE lower(a.title) = lower(v.title));

-- ให้เลข id ถัดไปไม่ชนกับที่มีอยู่
SELECT setval(pg_get_serial_sequence('anime', 'id'), (SELECT MAX(id) FROM anime));

-- ตรวจผล
SELECT count(*) AS total, count(poster) AS with_poster FROM anime;
`;
  writeFileSync(OUT, sql);
  console.log(`เสร็จ: เขียน ${rows.length} เรื่องลง ${OUT}`);
}

main().catch((e) => { console.error(e.message); process.exit(1); });
