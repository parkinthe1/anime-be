#!/usr/bin/env bash
# ทดสอบ API แบบเร็ว ๆ ด้วย curl: ./scripts/smoke-test.sh http://localhost:8080
# ต้องตั้ง ADMIN_EMAIL / ADMIN_PASSWORD ให้ server แล้ว และส่งไว้ใน env ตอนรันสคริปต์นี้ด้วย
#   ADMIN_EMAIL=admin@aniwave.test ADMIN_PASSWORD=xxxx ./scripts/smoke-test.sh https://xxx.onrender.com
BASE="${1:-http://localhost:8080}"
EMAIL="smoke$(date +%s)@test.dev"
PASS="1234"

check() { # check "ชื่อ" รหัสที่คาด รหัสที่ได้
  if [ "$2" = "$3" ]; then echo "PASS  $1 ($3)"; else echo "FAIL  $1 (คาด $2 ได้ $3)"; FAILED=1; fi
}
code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
json() { curl -s "$@"; }

check "guest ดูรายการอนิเมะ"            200 "$(code "$BASE/api/anime")"
check "guest ดูรายละเอียด"              200 "$(code "$BASE/api/anime/1")"
check "ไม่พบอนิเมะ"                     404 "$(code "$BASE/api/anime/999999")"
check "guest เข้า /api/me"              401 "$(code "$BASE/api/me")"
check "guest เข้า wishlist"             401 "$(code "$BASE/api/me/wishlist")"
check "guest เข้า admin"                401 "$(code "$BASE/api/admin/stats")"

check "สมัครสมาชิก"                     201 "$(code -X POST "$BASE/api/auth/register" -H 'Content-Type: application/json' -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"displayName\":\"Smoke\"}")"
check "สมัครซ้ำ (อีเมลซ้ำ)"             409 "$(code -X POST "$BASE/api/auth/register" -H 'Content-Type: application/json' -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"displayName\":\"Smoke\"}")"
check "login รหัสผิด"                   401 "$(code -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' -d "{\"email\":\"$EMAIL\",\"password\":\"wrong\"}")"

TOKEN=$(json -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\"}" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
AUTH="Authorization: Bearer $TOKEN"
[ -n "$TOKEN" ] && echo "PASS  login ได้ token" || { echo "FAIL  login ไม่ได้ token"; FAILED=1; }

check "member ดูโปรไฟล์"                200 "$(code "$BASE/api/me" -H "$AUTH")"
check "member ให้คะแนน"                 200 "$(code -X PUT "$BASE/api/anime/1/vote" -H "$AUTH" -H 'Content-Type: application/json' -d '{"score":9}')"
check "คะแนนเกิน 10"                    400 "$(code -X PUT "$BASE/api/anime/1/vote" -H "$AUTH" -H 'Content-Type: application/json' -d '{"score":11}')"
check "member เขียนรีวิว"               201 "$(code -X POST "$BASE/api/anime/1/reviews" -H "$AUTH" -H 'Content-Type: application/json' -d '{"text":"สนุกมาก ดูแล้วติดจนต้องดูต่อ"}')"
check "stats ของเรื่องที่ถูกโหวต"        200 "$(code "$BASE/api/anime/1/stats")"
check "member ให้คะแนนเรื่องจาก TENRAI" 200 "$(code -X PUT "$BASE/api/anime/52991/vote" -H "$AUTH" -H 'Content-Type: application/json' -d '{"score":8}')"
check "id ไม่ใช่เลข"                    400 "$(code -X PUT "$BASE/api/anime/abc/vote" -H "$AUTH" -H 'Content-Type: application/json' -d '{"score":8}')"
check "member เพิ่ม wishlist"           200 "$(code -X PUT "$BASE/api/me/wishlist/1" -H "$AUTH")"
check "member เพิ่ม wishlist (TENRAI)"  200 "$(code -X PUT "$BASE/api/me/wishlist/52991" -H "$AUTH")"
check "member ดู wishlist"              200 "$(code "$BASE/api/me/wishlist" -H "$AUTH")"
check "member ลบ wishlist (TENRAI)"     204 "$(code -X DELETE "$BASE/api/me/wishlist/52991" -H "$AUTH")"
check "member ลบ wishlist"              204 "$(code -X DELETE "$BASE/api/me/wishlist/1" -H "$AUTH")"
check "member เข้า admin ได้ 403"       403 "$(code "$BASE/api/admin/stats" -H "$AUTH")"
check "member แก้ข้อมูลอนิเมะ ได้ 403"  403 "$(code -X PUT "$BASE/api/admin/anime-overrides/1" -H "$AUTH" -H 'Content-Type: application/json' -d '{"titleTh":"x"}')"
check "guest ดูข้อมูลที่ admin แก้"     200 "$(code "$BASE/api/anime-overrides")"
check "token ปลอม"                      401 "$(code "$BASE/api/me" -H 'Authorization: Bearer fake.token.value')"

if [ -n "$ADMIN_EMAIL" ] && [ -n "$ADMIN_PASSWORD" ]; then
  ATOKEN=$(json -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
  AAUTH="Authorization: Bearer $ATOKEN"
  check "admin ดู stats"                200 "$(code "$BASE/api/admin/stats" -H "$AAUTH")"
  check "admin ดูรายชื่อสมาชิก"         200 "$(code "$BASE/api/admin/members" -H "$AAUTH")"
  check "admin แก้ข้อมูล ทุกช่องว่าง"   400 "$(code -X PUT "$BASE/api/admin/anime-overrides/52991" -H "$AAUTH" -H 'Content-Type: application/json' -d '{"titleTh":""}')"
  check "admin แก้ข้อมูล โปสเตอร์ผิด"   400 "$(code -X PUT "$BASE/api/admin/anime-overrides/52991" -H "$AAUTH" -H 'Content-Type: application/json' -d '{"poster":"javascript:alert(1)"}')"
  check "admin แก้ชื่อไทย"              200 "$(code -X PUT "$BASE/api/admin/anime-overrides/52991" -H "$AAUTH" -H 'Content-Type: application/json' -d '{"titleTh":"ทดสอบ"}')"
  check "admin ลบที่แก้ (กลับไปใช้ TENRAI)" 204 "$(code -X DELETE "$BASE/api/admin/anime-overrides/52991" -H "$AAUTH")"
else
  echo "SKIP  ส่วน admin (ไม่ได้ตั้ง ADMIN_EMAIL / ADMIN_PASSWORD)"
fi

[ -z "$FAILED" ] && echo "ผ่านทั้งหมด" || { echo "มีข้อที่ไม่ผ่าน"; exit 1; }
