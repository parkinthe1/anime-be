# MusicKen API (Backend — Go)

Backend ของเว็บเพลย์ลิสต์เพลง MusicKen · **Go (net/http) → PostgreSQL บน Supabase** · deploy บน **Render** · FE อยู่บน **Vercel**
รายละเอียด request/response ทุก endpoint อยู่ที่ `docs/api-contract.md`

## โครงสร้างโปรเจกต์

```
main.go              จุดเริ่มต้น: อ่าน config → ต่อ DB → เปิด server
config/              อ่าน environment variable
database/            เชื่อม Supabase (pgx) + สร้าง admin ตั้งต้น
models/              รูปร่างข้อมูล (Member, Artist, Song, Playlist) ชื่อ field เป็น camelCase
auth/                bcrypt (รหัสผ่าน) + JWT (token)
middleware/          RequireLogin / RequireAdmin (ตรวจสิทธิ์), CORS, Recover, Logger
router/              ประกาศเส้นทางทั้งหมด + ระบุว่าเส้นไหนต้องมีสิทธิ์อะไร
handlers/            ตัวจัดการแต่ละ endpoint แยกไฟล์ตามกลุ่ม (auth / me / song / artist / playlist / admin)
response/            ตอบ JSON / error รูปแบบเดียวกัน { "error": "..." }
db/schema.sql        สร้างตาราง   db/seed.sql  เพลงตัวอย่าง   db/drop-old-anime.sql  ลบตารางอนิเมะเก่า (ถ้าใช้ DB เดิม)
scripts/import-itunes.mjs  นำเข้าเพลงจริงจาก iTunes ผ่าน Go     scripts/smoke-test.sh  ทดสอบ API ด้วย curl
```

## สิทธิ์: guest / member / admin

| กลุ่ม | ทำอะไรได้ | ถ้าสิทธิ์ไม่พอ |
|---|---|---|
| guest | สมัคร/login ดูเพลง ศิลปิน และเพลย์ลิสต์สาธารณะ | – |
| member | แก้โปรไฟล์/เปลี่ยนรหัสผ่าน สร้างและจัดการเพลย์ลิสต์ของตัวเอง (เพิ่ม/ลบ/จัดลำดับเพลง) ถูกใจเพลย์ลิสต์ บันทึกและดูประวัติการฟัง | 401 |
| admin | จัดการเพลง ศิลปิน สมาชิก (เปลี่ยนบทบาท) และลบเพลย์ลิสต์ที่ไม่เหมาะสม ดูสถิติ | 401 (ไม่ login) / 403 (เป็น member) |

**การตรวจสิทธิ์ทำที่ Go ทุกครั้ง:** middleware อ่านสมาชิกจากฐานข้อมูลตาม token ทุก request
ต่อให้แก้ localStorage ฝั่งหน้าเว็บเป็น admin ก็ยิง `/api/admin/*` ไม่ผ่าน (ได้ 403) และ `member_id` ใช้จาก token เท่านั้น ไม่รับจาก body
เพลย์ลิสต์ของคนอื่นตอบ 404 (ไม่บอกว่ามีอยู่)

## 1) ตั้งฐานข้อมูลบน Supabase

1. สร้าง Project ใน Supabase (จดรหัสผ่านฐานข้อมูลไว้)
2. ถ้าใช้ฐานข้อมูลเดิมของโปรเจกต์อนิเมะ: **SQL Editor** รัน `db/drop-old-anime.sql` ก่อน (ตาราง `members` ใช้ต่อได้)
3. **SQL Editor → New query** วาง `db/schema.sql` แล้ว Run (ถ้าอยากได้เพลงตัวอย่างเร็ว ๆ รัน `db/seed.sql` ด้วย)
4. **Project Settings → Database → Connection string** เลือก **Session pooler** (พอร์ต 5432) แล้วคัดลอก URI
   - Render เชื่อมผ่าน IPv4 จึงต้องใช้ pooler ถ้าใช้ "Direct connection" จะต่อไม่ติด
   - ถ้ารหัสผ่านมีอักขระพิเศษ (`@ : / # ?`) ต้อง URL-encode ก่อนใส่ใน URL

## 2) รันในเครื่อง

ต้องมี Go 1.22 ขึ้นไป

```bash
cp .env.example .env        # แล้วแก้ DATABASE_URL, JWT_SECRET, ADMIN_PASSWORD
go mod tidy                 # ครั้งแรกครั้งเดียว: ตรวจ dependency และอัปเดต go.sum (ต้อง commit go.sum ด้วย)
go vet ./... && go run .
curl localhost:8080/api/songs
```

บัญชี admin: ถ้าตั้ง `ADMIN_EMAIL` และ `ADMIN_PASSWORD` ตอน server เปิดจะสร้างให้อัตโนมัติถ้ายังไม่มี สมาชิกทั่วไปสมัครผ่านหน้า Register

## 3) นำเข้าเพลงจริงจาก iTunes (ทำครั้งเดียว)

สคริปต์ดึงเพลงจาก iTunes Search API (ไม่ต้องใช้ key) แล้วส่งเข้า Go ที่ `POST /api/admin/songs` จึงผ่าน Go → Supabase เหมือนข้อมูลอื่น
ใช้เฉพาะรูปปกและเสียงตัวอย่าง 30 วินาทีที่ iTunes ให้ลิงก์ ไม่เก็บไฟล์เสียง

```bash
API=http://localhost:8080 ADMIN_EMAIL=admin@musicken.test ADMIN_PASSWORD=รหัสแอดมิน \
  node scripts/import-itunes.mjs                      # ใช้รายการศิลปินตั้งต้น
node scripts/import-itunes.mjs "coldplay" "bts"       # หรือระบุคำค้นเอง (ตั้ง env เหมือนข้างบน)
```
รันซ้ำได้ เพลงที่นำเข้าแล้วจะถูกข้าม (ตอบ 409) ถ้าจะนำเข้าเข้า Render ให้ตั้ง `API=https://<ชื่อ>.onrender.com`

## 4) Deploy บน Render

1. push โค้ดโฟลเดอร์นี้ขึ้น GitHub (repo ของ backend; **ห้าม commit `.env`**)
2. Render → **New → Web Service** → เชื่อม repo นั้น
3. ตั้งค่า
   - Language: **Go**
   - Build Command: `go build -o app .`
   - Start Command: `./app`
   - ถ้า build ฟ้อง Go version ให้เพิ่ม Environment Variable `GO_VERSION` (เช่น `1.22.5`)
4. Environment Variables: `DATABASE_URL`, `JWT_SECRET`, `CORS_ORIGINS`, `ADMIN_EMAIL`, `ADMIN_PASSWORD` (ไม่ต้องตั้ง `PORT` Render ตั้งให้เอง)
   - `CORS_ORIGINS` = `http://localhost:3000,https://<โดเมน-vercel-ของทีม>.vercel.app`
5. Deploy แล้วเปิด `https://<ชื่อ>.onrender.com/healthz` ควรได้ `{"status":"ok"}`

> แพ็กเกจฟรีของ Render จะหลับเมื่อไม่มีคนใช้ request แรกอาจช้า 30–60 วินาที ก่อนสาธิตให้เปิดเว็บทิ้งไว้ก่อน

## 5) ต่อ FE บน Vercel

Vercel → Project → Settings → Environment Variables เพิ่ม `REACT_APP_API_URL` = `https://<ชื่อ>.onrender.com` (ไม่มี `/` ท้าย) แล้ว **Redeploy**
(ตัวแปรของ CRA ถูกฝังตอน build ถ้าเพิ่มทีหลังต้อง redeploy)

## 6) ทดสอบ

```bash
ADMIN_EMAIL=admin@musicken.test ADMIN_PASSWORD=รหัสแอดมิน ./scripts/smoke-test.sh http://localhost:8080
```
สคริปต์สมัครสมาชิกทดสอบ สร้างเพลย์ลิสต์ ทดลองแก้ของคนอื่น (ต้องได้ 404) และยิง `/api/admin/*` ด้วย member (ต้องได้ 403)
