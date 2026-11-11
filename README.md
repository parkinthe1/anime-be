# AniWave API (Backend — Go)

Backend ของเว็บอนิเมะ AniWave สร้างตามสัญญา API ของ FE (`docs/api-contract.md` ใน repo anime-fe)
**Go (net/http) → PostgreSQL บน Supabase** · deploy บน **Render** · FE อยู่บน **Vercel**

## โครงสร้างโปรเจกต์

```
main.go              จุดเริ่มต้น: อ่าน config → ต่อ DB → เปิด server
config/              อ่าน environment variable
database/            เชื่อม Supabase (pgx) + สร้าง admin ตั้งต้น
models/              รูปร่างข้อมูล (Member, Anime) ชื่อ field เป็น camelCase
auth/                bcrypt (รหัสผ่าน) + JWT (token)
middleware/          RequireLogin / RequireAdmin (ตรวจสิทธิ์), CORS, Recover, Logger
router/              ประกาศเส้นทางทั้งหมด + ระบุว่าเส้นไหนต้องมีสิทธิ์อะไร
handlers/            ตัวจัดการแต่ละ endpoint แยกไฟล์ตามกลุ่ม
response/            ตอบ JSON / error รูปแบบเดียวกัน { "error": "..." }
db/schema.sql        สร้างตาราง   db/seed.sql  ข้อมูลอนิเมะตั้งต้น 12 เรื่อง
scripts/smoke-test.sh  ทดสอบ API ด้วย curl
```

## Endpoint และสิทธิ์

| กลุ่ม | Endpoint | ถ้าสิทธิ์ไม่พอ |
|---|---|---|
| guest | `POST /api/auth/register` · `POST /api/auth/login` · `GET /api/anime` (`?q=`) · `GET /api/anime/{id}` · `GET /api/anime/{id}/reviews` | – |
| member | `GET/PUT /api/me` · `PUT /api/me/password` · `POST /api/anime/{id}/reviews` · `PUT /api/anime/{id}/vote` · `GET /api/me/wishlist` · `PUT/DELETE /api/me/wishlist/{id}` | 401 |
| admin | `GET /api/admin/stats` · `GET /api/admin/members` · `PUT /api/admin/members/{id}/role` · `GET/POST /api/admin/anime` · `PUT/DELETE /api/admin/anime/{id}` | 401 (ไม่ login) / 403 (เป็น member) |

รายละเอียด request/response อยู่ใน `docs/api-contract.md` ของ FE (ทำตามนั้นทุกจุด) และมี `GET /healthz` สำหรับเช็กว่า server ตื่นอยู่

**การตรวจสิทธิ์ทำที่ Go ทุกครั้ง:** middleware อ่านสมาชิกจากฐานข้อมูลตาม token ทุก request
ต่อให้แก้ localStorage ฝั่งหน้าเว็บเป็น admin ก็ยิง `/api/admin/*` ไม่ผ่าน (ได้ 403) และ `member_id` ใช้จาก token เท่านั้น ไม่รับจาก body

## 1) ตั้งฐานข้อมูลบน Supabase

1. สร้าง Project ใน Supabase (จดรหัสผ่านฐานข้อมูลไว้)
2. **SQL Editor → New query** วางเนื้อหา `db/schema.sql` แล้ว Run จากนั้นทำซ้ำกับ `db/seed.sql`
3. **Project Settings → Database → Connection string** เลือก **Session pooler** (พอร์ต 5432) แล้วคัดลอก URI
   - Render เชื่อมผ่าน IPv4 จึงต้องใช้ pooler ถ้าใช้ "Direct connection" จะต่อไม่ติด
   - ถ้ารหัสผ่านมีอักขระพิเศษ (`@ : / # ?`) ต้อง URL-encode ก่อนใส่ใน URL

## 2) รันในเครื่อง

ต้องมี Go 1.22 ขึ้นไป

```bash
cp .env.example .env        # แล้วแก้ DATABASE_URL, JWT_SECRET, ADMIN_PASSWORD
go mod tidy                 # ครั้งแรกครั้งเดียว: โหลด dependency และสร้าง go.sum (ต้อง commit go.sum ด้วย)
go run .
curl localhost:8080/api/anime
```

ต่อกับ FE โดยไม่ต้องแก้ FE: ตั้ง `PORT=4000` ใน `.env` แล้วรัน `go run .` (แทน `npm run mock`) เพราะ `proxy` ใน package.json ของ FE ชี้พอร์ต 4000 อยู่แล้ว
หรือใช้พอร์ต 8080 แล้วตั้ง `REACT_APP_API_URL=http://localhost:8080` ในไฟล์ `.env` ของ FE (ต้องรัน `npm start` ใหม่)

บัญชี admin: ถ้าตั้ง `ADMIN_EMAIL` และ `ADMIN_PASSWORD` ตอน server เปิดจะสร้างให้อัตโนมัติถ้ายังไม่มี (รหัสผ่านไม่ต้องอยู่ในโค้ด)
สมาชิกทั่วไปสมัครผ่านหน้า Register ปกติ

## 3) Deploy บน Render

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

> แพ็กเกจฟรีของ Render จะหลับเมื่อไม่มีคนใช้ request แรกอาจช้า 30–60 วินาที ก่อนอัดคลิปหรือสาธิตให้เปิดเว็บทิ้งไว้ก่อน

## 4) ต่อ FE บน Vercel

Vercel → Project → Settings → Environment Variables เพิ่ม `REACT_APP_API_URL` = `https://<ชื่อ>.onrender.com` (ไม่มี `/` ท้าย) แล้ว **Redeploy**
(ตัวแปรของ CRA ถูกฝังตอน build ถ้าเพิ่มทีหลังต้อง redeploy)

## 5) ทดสอบ

```bash
ADMIN_EMAIL=admin@aniwave.test ADMIN_PASSWORD=รหัสที่ตั้ง ./scripts/smoke-test.sh https://<ชื่อ>.onrender.com
```

สคริปต์ตรวจ guest/member/admin (401/403), สมัครซ้ำ (409), ข้อมูลไม่ถูกต้อง (400) และ flow หลักของสมาชิก

## ค่าที่ตั้งใจเลือก

- รหัสผ่านขั้นต่ำ 4 ตัวตามสัญญาของ FE ถ้าจะเพิ่มความเข้ม แก้ `minPasswordLen` ใน `handlers/handler.go` และข้อความฝั่ง FE ให้ตรงกัน
- token JWT อายุ 24 ชั่วโมง (แก้ใน `main.go`) FE จะออกจากระบบอัตโนมัติเมื่อได้ 401
- เปิด Row Level Security ทุกตารางใน `schema.sql` เพื่อไม่ให้คนนอกอ่านตาราง `members` (มี password_hash) ผ่าน Data API ของ Supabase ได้
