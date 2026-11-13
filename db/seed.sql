-- ข้อมูลตั้งต้นสำหรับทดลอง (ศิลปินและเพลงสมมติ ไม่ใช่ข้อมูลจริง) รันหลัง schema.sql
-- ของจริงนำเข้าจาก iTunes ด้วย scripts/import-itunes.mjs
INSERT INTO artists (name, bio) VALUES
  ('Demo Band',   'วงตัวอย่างสำหรับทดสอบระบบ'),
  ('Sample Voice', 'ศิลปินตัวอย่างแนวป๊อป'),
  ('Test Beats',   'โปรดิวเซอร์ตัวอย่างแนวอิเล็กทรอนิกส์')
ON CONFLICT DO NOTHING;

INSERT INTO songs (title, artist_id, album, genre, year, duration_sec)
SELECT v.title, a.id, v.album, v.genre, v.year, v.dur
FROM (VALUES
  ('Morning Demo',  'Demo Band',    'First Take',   'Rock', 2021, 215),
  ('Night Sample',  'Demo Band',    'First Take',   'Rock', 2021, 198),
  ('Hello Test',    'Sample Voice', 'Sound Check',  'Pop',  2022, 187),
  ('Fake Love',     'Sample Voice', 'Sound Check',  'Pop',  2022, 204),
  ('Loop One',      'Test Beats',   'Loops',        'Electronic', 2023, 240),
  ('Loop Two',      'Test Beats',   'Loops',        'Electronic', 2023, 252)
) AS v(title, artist, album, genre, year, dur)
JOIN artists a ON lower(a.name) = lower(v.artist)
WHERE NOT EXISTS (SELECT 1 FROM songs s WHERE s.title = v.title AND s.artist_id = a.id);
