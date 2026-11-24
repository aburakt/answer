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

-- ib: Keycloak (connector-basic, provider 'basic') hesaplarını LDAP girişine
-- (provider 'ldap') bağlar. Answer, bilinmeyen bir dış kimlik aynı e-postayla
-- gelirse girişi reddeder; bu yüzden her kullanıcıya sicili external_id olan
-- bir 'ldap' satırı eklenir. Eklentide "Dış kimlik alanı" sicili
-- veren alan olmalı (ör. sAMAccountName). 'basic' satırlarına dokunulmaz, geri
-- dönüşte Keycloak çalışmaya devam eder. İki kez çalıştırılırsa yeni satır eklemez.
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

-- 2) Eşleme.
INSERT INTO user_external_login (created_at, updated_at, user_id, provider, external_id, meta_info)
SELECT now(), now(), b.user_id, 'ldap', b.meta_info::json->>'preferred_username', '{}'
FROM user_external_login b
WHERE b.provider = 'basic'
  AND b.meta_info::json->>'preferred_username' ~ '^[0-9A-Za-z]{1,32}$'
  AND NOT EXISTS (
    SELECT 1 FROM user_external_login l
    WHERE l.provider = 'ldap'
      AND (l.user_id = b.user_id OR l.external_id = b.meta_info::json->>'preferred_username')
  );

-- Sonuç: her kullanıcının bir 'ldap' satırı olmalı, external_id = sicil.
SELECT user_id, external_id FROM user_external_login WHERE provider = 'ldap' ORDER BY user_id;

COMMIT;
