#!/usr/bin/env bash
# ทดสอบ API แบบเร็ว ๆ ด้วย curl: ./scripts/smoke-test.sh http://localhost:8080
# ต้องตั้ง ADMIN_EMAIL / ADMIN_PASSWORD ให้ server แล้ว และส่งไว้ใน env ตอนรันสคริปต์นี้ด้วย
#   ADMIN_EMAIL=admin@musicken.test ADMIN_PASSWORD=xxxx ./scripts/smoke-test.sh https://xxx.onrender.com
BASE="${1:-http://localhost:8080}"
EMAIL="smoke$(date +%s)@test.dev"
EMAIL2="smoke2$(date +%s)@test.dev"
PASS="1234"
FAILED=0
JSONH='Content-Type: application/json'

check() { # check "ชื่อ" รหัสที่คาด รหัสที่ได้
  if [ "$2" = "$3" ]; then echo "PASS  $1 ($3)"; else echo "FAIL  $1 (คาด $2 ได้ $3)"; FAILED=1; fi
}
code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
json() { curl -s "$@"; }
login_token() { json -X POST "$BASE/api/auth/login" -H "$JSONH" -d "{\"email\":\"$1\",\"password\":\"$2\"}" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p'; }

echo "== guest =="
check "healthz"                          200 "$(code "$BASE/healthz")"
check "guest ดูรายการเพลง"               200 "$(code "$BASE/api/songs")"
check "guest ค้นเพลง"                    200 "$(code "$BASE/api/songs?q=a&limit=5")"
check "guest ดูแนวเพลง"                  200 "$(code "$BASE/api/genres")"
check "guest ดูศิลปิน"                   200 "$(code "$BASE/api/artists")"
check "guest ดูเพลย์ลิสต์สาธารณะ"        200 "$(code "$BASE/api/playlists")"
check "ไม่พบเพลง"                        404 "$(code "$BASE/api/songs/999999999")"
check "id ไม่ใช่เลข"                     404 "$(code "$BASE/api/songs/abc")"
check "guest เข้า /api/me"               401 "$(code "$BASE/api/me")"
check "guest เข้าเพลย์ลิสต์ของฉัน"       401 "$(code "$BASE/api/me/playlists")"
check "guest เข้า admin"                 401 "$(code "$BASE/api/admin/stats")"

echo "== สมาชิก =="
check "สมัครสมาชิก"                      201 "$(code -X POST "$BASE/api/auth/register" -H "$JSONH" -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"displayName\":\"Smoke\"}")"
check "สมัครซ้ำ (อีเมลซ้ำ)"              409 "$(code -X POST "$BASE/api/auth/register" -H "$JSONH" -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"displayName\":\"Smoke\"}")"
check "login รหัสผิด"                    401 "$(code -X POST "$BASE/api/auth/login" -H "$JSONH" -d "{\"email\":\"$EMAIL\",\"password\":\"wrong\"}")"
TOKEN=$(login_token "$EMAIL" "$PASS"); AUTH="Authorization: Bearer $TOKEN"
[ -n "$TOKEN" ] && echo "PASS  login ได้ token" || { echo "FAIL  login ไม่ได้ token"; FAILED=1; }
check "member ดูโปรไฟล์"                 200 "$(code "$BASE/api/me" -H "$AUTH")"
check "member แก้ชื่อ"                   200 "$(code -X PUT "$BASE/api/me" -H "$AUTH" -H "$JSONH" -d '{"displayName":"Smoke Two"}')"
check "เปลี่ยนรหัส (รหัสเดิมผิด)"        400 "$(code -X PUT "$BASE/api/me/password" -H "$AUTH" -H "$JSONH" -d '{"currentPassword":"nope","newPassword":"5678"}')"
check "เปลี่ยนรหัสผ่าน"                  200 "$(code -X PUT "$BASE/api/me/password" -H "$AUTH" -H "$JSONH" -d "{\"currentPassword\":\"$PASS\",\"newPassword\":\"5678\"}")"
check "login ด้วยรหัสเก่าไม่ได้"          401 "$(code -X POST "$BASE/api/auth/login" -H "$JSONH" -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\"}")"
PASS="5678"; TOKEN=$(login_token "$EMAIL" "$PASS"); AUTH="Authorization: Bearer $TOKEN"
check "login ด้วยรหัสใหม่"               200 "$(code -X POST "$BASE/api/auth/login" -H "$JSONH" -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\"}")"

echo "== เพลย์ลิสต์ =="
check "สร้างเพลย์ลิสต์ ชื่อว่าง"          400 "$(code -X POST "$BASE/api/me/playlists" -H "$AUTH" -H "$JSONH" -d '{"name":"  "}')"
PL=$(json -X POST "$BASE/api/me/playlists" -H "$AUTH" -H "$JSONH" -d '{"name":"Smoke list","description":"test","isPublic":true}')
PID=$(echo "$PL" | sed -n 's/.*"id":\([0-9]*\).*/\1/p')
[ -n "$PID" ] && echo "PASS  สร้างเพลย์ลิสต์ id=$PID" || { echo "FAIL  สร้างเพลย์ลิสต์ไม่ได้: $PL"; FAILED=1; }
SONG=$(json "$BASE/api/songs?limit=2" | sed -n 's/.*"items":\[{"id":\([0-9]*\).*/\1/p')
if [ -n "$SONG" ] && [ -n "$PID" ]; then
  check "เพิ่มเพลงลงลิสต์"                 204 "$(code -X PUT "$BASE/api/me/playlists/$PID/songs/$SONG" -H "$AUTH")"
  check "เพิ่มซ้ำไม่ error"                204 "$(code -X PUT "$BASE/api/me/playlists/$PID/songs/$SONG" -H "$AUTH")"
  check "เพิ่มเพลงที่ไม่มี"                404 "$(code -X PUT "$BASE/api/me/playlists/$PID/songs/999999999" -H "$AUTH")"
  check "จัดลำดับ (ตรงกับที่มี)"           200 "$(code -X PUT "$BASE/api/me/playlists/$PID/order" -H "$AUTH" -H "$JSONH" -d "{\"songIds\":[$SONG]}")"
  check "จัดลำดับ (เพลงไม่ครบ)"            400 "$(code -X PUT "$BASE/api/me/playlists/$PID/order" -H "$AUTH" -H "$JSONH" -d '{"songIds":[]}')"
  check "บันทึกการฟัง"                     204 "$(code -X POST "$BASE/api/songs/$SONG/play" -H "$AUTH")"
  check "ดูประวัติการฟัง"                  200 "$(code "$BASE/api/me/history" -H "$AUTH")"
else
  echo "SKIP  ยังไม่มีเพลงในฐานข้อมูล (รัน db/seed.sql หรือ scripts/import-itunes.mjs ก่อน)"
fi
check "guest ดูลิสต์สาธารณะได้"          200 "$(code "$BASE/api/playlists/$PID")"
check "ถูกใจเพลย์ลิสต์"                  200 "$(code -X PUT "$BASE/api/playlists/$PID/like" -H "$AUTH")"
check "ดูเพลย์ลิสต์ที่ถูกใจ"             200 "$(code "$BASE/api/me/likes" -H "$AUTH")"
check "guest ถูกใจไม่ได้"                401 "$(code -X PUT "$BASE/api/playlists/$PID/like")"

echo "== สิทธิ์ข้ามผู้ใช้ =="
check "สมัครสมาชิกคนที่ 2"               201 "$(code -X POST "$BASE/api/auth/register" -H "$JSONH" -d "{\"email\":\"$EMAIL2\",\"password\":\"1234\",\"displayName\":\"Smoke B\"}")"
TOKEN2=$(login_token "$EMAIL2" "1234"); AUTH2="Authorization: Bearer $TOKEN2"
check "คนอื่นแก้ลิสต์ของเราไม่ได้"       404 "$(code -X PUT "$BASE/api/me/playlists/$PID" -H "$AUTH2" -H "$JSONH" -d '{"name":"hack"}')"
check "คนอื่นลบลิสต์ของเราไม่ได้"        404 "$(code -X DELETE "$BASE/api/me/playlists/$PID" -H "$AUTH2")"
check "คนอื่นเพิ่มเพลงลงลิสต์เราไม่ได้"   404 "$(code -X PUT "$BASE/api/me/playlists/$PID/songs/${SONG:-1}" -H "$AUTH2")"
check "member เข้า admin ได้ 403"        403 "$(code "$BASE/api/admin/stats" -H "$AUTH")"
check "member เพิ่มเพลงเป็น admin ไม่ได้" 403 "$(code -X POST "$BASE/api/admin/songs" -H "$AUTH" -H "$JSONH" -d '{"title":"x","artistName":"y"}')"
check "token ปลอม"                       401 "$(code "$BASE/api/me" -H 'Authorization: Bearer abc.def.ghi')"

echo "== admin =="
if [ -n "$ADMIN_EMAIL" ] && [ -n "$ADMIN_PASSWORD" ]; then
  ATOKEN=$(login_token "$ADMIN_EMAIL" "$ADMIN_PASSWORD"); AAUTH="Authorization: Bearer $ATOKEN"
  check "admin ดู stats"                   200 "$(code "$BASE/api/admin/stats" -H "$AAUTH")"
  check "admin ดูสมาชิก"                   200 "$(code "$BASE/api/admin/members" -H "$AAUTH")"
  check "admin เพิ่มเพลง ชื่อว่าง"         400 "$(code -X POST "$BASE/api/admin/songs" -H "$AAUTH" -H "$JSONH" -d '{"title":"","artistName":"x"}')"
  check "admin เพิ่มเพลง ลิงก์ผิด"         400 "$(code -X POST "$BASE/api/admin/songs" -H "$AAUTH" -H "$JSONH" -d '{"title":"t","artistName":"x","coverUrl":"javascript:alert(1)"}')"
  NEWSONG=$(json -X POST "$BASE/api/admin/songs" -H "$AAUTH" -H "$JSONH" -d "{\"title\":\"Smoke Song\",\"artistName\":\"Smoke Artist $(date +%s)\",\"genre\":\"Pop\"}")
  SID=$(echo "$NEWSONG" | sed -n 's/.*"id":\([0-9]*\).*/\1/p')
  [ -n "$SID" ] && echo "PASS  admin เพิ่มเพลง id=$SID" || { echo "FAIL  admin เพิ่มเพลงไม่ได้: $NEWSONG"; FAILED=1; }
  check "admin แก้เพลง"                    200 "$(code -X PUT "$BASE/api/admin/songs/$SID" -H "$AAUTH" -H "$JSONH" -d '{"title":"Smoke Song 2","artistName":"Smoke Artist X"}')"
  check "admin ลบเพลง"                     204 "$(code -X DELETE "$BASE/api/admin/songs/$SID" -H "$AAUTH")"
  check "ลบเพลงซ้ำ ได้ 404"                404 "$(code -X DELETE "$BASE/api/admin/songs/$SID" -H "$AAUTH")"
  check "admin ลบเพลย์ลิสต์ของคนอื่น"      204 "$(code -X DELETE "$BASE/api/admin/playlists/$PID" -H "$AAUTH")"
else
  echo "SKIP  ไม่ได้ตั้ง ADMIN_EMAIL / ADMIN_PASSWORD ใน env"
fi

[ "$FAILED" = "0" ] && echo "ผ่านทั้งหมด" || { echo "มีข้อที่ไม่ผ่าน"; exit 1; }
