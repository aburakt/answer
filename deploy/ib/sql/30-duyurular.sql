--
-- Licensed to the Apache Software Foundation (ASF) under one
-- or more contributor license agreements.  See the NOTICE file
-- distributed with this work for additional information
-- regarding copyright ownership.  The ASF licenses this file
-- to you under the Apache License, Version 2.0 (the
-- "License"); you may not use this file except in compliance
-- with the License.  You may obtain a copy of the License at
--
--   http://www.apache.org/licenses/LICENSE-2.0
--
-- Unless required by applicable law or agreed to in writing,
-- software distributed under the License is distributed on an
-- "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
-- KIND, either express or implied.  See the License for the
-- specific language governing permissions and limitations
-- under the License.
--

-- ib: eski duyuruları yeni "Duyuru" işaretine çevirir. İsteğe bağlıdır.
-- 1.6.0'da duyuru, soruyu kapatıp sabitleyerek yapılıyordu ("kapatıldı"
-- metinleri "DUYURU" diye çevrilmişti). Yeni sürümde "Duyuru Yap" işlemi var;
-- bu betik hem kapatılmış hem sabitlenmiş soruları duyuru olarak işaretler.
-- Duyuru tarihi, sorunun kapatıldığı tarihtir. Sorular kapalı kalır (cevap
-- yazılamaz), yalnız üstlerinde "kapatıldı" yerine duyuru bandı görünür.
-- Tekrar çalıştırılabilir. Önce hangi soruların etkileneceğine bakın:
--   SELECT id, title FROM question WHERE status = 2 AND pin = 2;
BEGIN;
INSERT INTO meta (created_at, updated_at, object_id, key, value)
SELECT COALESCE(c.created_at, q.updated_at, q.created_at), now(), q.id, 'ib.question.announcement', '1'
FROM question q
LEFT JOIN LATERAL (
    SELECT created_at FROM meta
    WHERE object_id = q.id AND key = 'question.close.reason'
    ORDER BY id DESC LIMIT 1
) c ON true
WHERE q.status = 2 AND q.pin = 2
  AND NOT EXISTS (
    SELECT 1 FROM meta m WHERE m.object_id = q.id AND m.key = 'ib.question.announcement'
  );
COMMIT;
