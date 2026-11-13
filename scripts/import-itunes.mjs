// นำเข้าเพลงจาก iTunes Search API เข้า MusicKen ผ่าน Go (ไม่เขียนลง DB ตรง)
//   iTunes Search API -> สคริปต์นี้ -> POST /api/admin/songs ของ Go -> Supabase
// ใช้ครั้งเดียวหรือเมื่ออยากเพิ่มเพลง (เพลงที่นำเข้าแล้วจะถูกข้าม เพราะ externalId ซ้ำ = 409)
//
// วิธีใช้ (ต้องมี Node 18 ขึ้นไป):
//   API=http://localhost:8080 ADMIN_EMAIL=admin@musicken.test ADMIN_PASSWORD=xxxx \
//   node scripts/import-itunes.mjs "coldplay" "taylor swift" "bruno mars" "billie eilish"
// ถ้าไม่ใส่คำค้น จะใช้รายการตั้งต้นด้านล่าง
//
// หมายเหตุ: ใช้เฉพาะรูปปกและเสียงตัวอย่าง 30 วินาทีที่ iTunes ให้ลิงก์มา ไม่ดาวน์โหลดหรือเก็บไฟล์เสียง

const API = (process.env.API || 'http://localhost:8080').replace(/\/$/, '');
const EMAIL = process.env.ADMIN_EMAIL;
const PASSWORD = process.env.ADMIN_PASSWORD;
const PER_TERM = Number(process.env.PER_TERM || 12);        // เพลงต่อคำค้น
const DEFAULT_TERMS = ['coldplay', 'taylor swift', 'bruno mars', 'billie eilish', 'ed sheeran', 'bts'];

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function goFetch(path, { method = 'GET', body, token } = {}) {
  const res = await fetch(API + path, {
    method,
    headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  });
  const data = res.status === 204 ? null : await res.json().catch(() => null);
  return { status: res.status, data };
}

// รูปปกจาก iTunes ขนาดเล็ก (100x100) ขยายเป็น 300x300 ได้โดยแก้ท้าย URL
const bigCover = (url) => (url ? url.replace(/\/\d+x\d+bb\./, '/300x300bb.') : null);

async function searchItunes(term) {
  const url = `https://itunes.apple.com/search?term=${encodeURIComponent(term)}&media=music&entity=song&limit=${PER_TERM}`;
  const res = await fetch(url);
  if (!res.ok) throw new Error(`iTunes ตอบกลับ ${res.status} (ค้น "${term}")`);
  const json = await res.json();
  return (json.results || []).filter((t) => t.trackName && t.artistName);
}

const toSong = (t) => ({
  title: t.trackName,
  artistName: t.artistName,
  album: t.collectionName || null,
  genre: t.primaryGenreName || null,
  year: t.releaseDate ? new Date(t.releaseDate).getUTCFullYear() : null,
  durationSec: t.trackTimeMillis ? Math.round(t.trackTimeMillis / 1000) : null,
  coverUrl: bigCover(t.artworkUrl100),
  previewUrl: t.previewUrl || null,
  externalId: t.trackId,
});

async function main() {
  if (!EMAIL || !PASSWORD) {
    console.error('ต้องตั้ง ADMIN_EMAIL และ ADMIN_PASSWORD (บัญชี admin ของ Go) ก่อนรัน');
    process.exit(1);
  }
  const login = await goFetch('/api/auth/login', { method: 'POST', body: { email: EMAIL, password: PASSWORD } });
  if (login.status !== 200) {
    console.error(`login admin ไม่ผ่าน (${login.status}) ตรวจ API / ADMIN_EMAIL / ADMIN_PASSWORD`);
    process.exit(1);
  }
  const token = login.data.token;

  const terms = process.argv.length > 2 ? process.argv.slice(2) : DEFAULT_TERMS;
  let added = 0, skipped = 0, failed = 0;

  for (const term of terms) {
    let tracks;
    try {
      tracks = await searchItunes(term);
    } catch (err) {
      console.error(String(err.message || err));
      failed++;
      continue;
    }
    console.log(`"${term}": พบ ${tracks.length} เพลง`);
    for (const t of tracks) {
      const r = await goFetch('/api/admin/songs', { method: 'POST', body: toSong(t), token });
      if (r.status === 201) added++;
      else if (r.status === 409) skipped++;           // นำเข้าไปแล้ว
      else { failed++; console.error(`  ข้าม "${t.trackName}": ${r.status} ${r.data?.error ?? ''}`); }
    }
    await sleep(3500);                                // iTunes จำกัดจำนวนครั้งต่อนาที เว้นจังหวะระหว่างคำค้น
  }
  console.log(`เสร็จ: เพิ่ม ${added} · ข้าม (มีแล้ว) ${skipped} · ผิดพลาด ${failed}`);
}

main().catch((e) => { console.error(e); process.exit(1); });
