-- ใช้เฉพาะถ้าใช้ฐานข้อมูลเดิมของโปรเจกต์อนิเมะ: ลบตารางอนิเมะเก่าทิ้ง (สมาชิก members ยังอยู่ ใช้ต่อได้)
DROP TABLE IF EXISTS anime_overrides, wishlist_items, votes, reviews, anime CASCADE;
