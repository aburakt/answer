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

-- ib: Keycloak (connector-basic) kayıtlarını giris-sso köprüsüne taşır.
-- Köprü de connector-basic ile bağlanır (provider 'basic' kalır); yalnız
-- external_id, Keycloak'ın `sub` UUID'si yerine sicil (köprünün `sub`'ı) olur.
-- Keycloak'ta user_id_json_path zaten preferred_username idiyse değişen satır
-- olmaz. Tek transaction; önce kontrol raporu, sonra yedek tablo, sonra güncelleme.
BEGIN;

-- 1) Rapor: sicili okunamayan ya da aynı sicile iki hesap bağlanan kayıtlar.
--    Bu iki sorgu satır döndürürse COMMIT etmeden durun ve elle çözün.
SELECT 'sicil_yok' AS sorun, id, user_id, external_id
FROM user_external_login
WHERE provider = 'basic'
  AND coalesce(meta_info::json->>'preferred_username', '') !~ '^[0-9A-Za-z]{1,32}$';

SELECT 'cift_sicil' AS sorun, meta_info::json->>'preferred_username' AS sicil,
       count(*) AS adet, string_agg(user_id::text, ',') AS kullanicilar
FROM user_external_login
WHERE provider = 'basic'
GROUP BY 1, 2
HAVING count(*) > 1;

-- 2) Yedek (tekrar çalıştırılırsa ilk yedek korunur).
CREATE TABLE IF NOT EXISTS ib_user_external_login_keycloak AS
SELECT * FROM user_external_login WHERE provider = 'basic';

-- 3) Eşleme.
UPDATE user_external_login
SET external_id = meta_info::json->>'preferred_username',
    updated_at  = now()
WHERE provider = 'basic'
  AND meta_info::json->>'preferred_username' ~ '^[0-9A-Za-z]{1,32}$'
  AND external_id <> meta_info::json->>'preferred_username';

-- Sonuç: her satırda external_id = sicil olmalı.
SELECT user_id, external_id FROM user_external_login WHERE provider = 'basic' ORDER BY user_id;

COMMIT;
