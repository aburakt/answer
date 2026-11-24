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

-- ib: v2.0.3 upgrade'inden sonra intranet ayarları.
-- Upgrade, v2.0.3'te `require_email_verification=true` ekliyor; connector'dan
-- gelen e-postalar zaten doğrulanmış sayıldığı için kapatılır. Varsayılan avatar
-- Gravatar'dı (intranette erişilemez): sistem avatarına çekilir.
-- Kayıt kapanır: kullanıcı hesapları ilk LDAP girişinde kendiliğinden açılır.
-- Şifreli giriş açık kalır, çünkü yönetici /users/admin'den onunla girer
-- (sunucu şifreli girişi yalnız yöneticiye izin verir). Ana renk kurum laciverti, üst bar
-- beyaz olur (logo koyu yazılıdır).
BEGIN;
UPDATE site_info SET content = jsonb_set(content::jsonb, '{require_email_verification}', 'false')::text
WHERE type = 'login';
UPDATE site_info SET content = jsonb_set(content::jsonb, '{default_avatar}', '"system"')::text
WHERE type = 'users_settings';
UPDATE site_info SET content = (content::jsonb
    || '{"allow_new_registrations": false, "allow_email_registrations": false, "allow_password_login": true}'::jsonb)::text
WHERE type = 'login';
UPDATE site_info SET content = jsonb_set(jsonb_set(content::jsonb,
    '{theme_config,default,primary_color}', '"#061c5f"'),
    '{theme_config,default,navbar_style}', '"#ffffff"')::text
WHERE type = 'theme';
-- Logo: uygulama amblemi ve yazıyı kendisi çizer, yazı rengi açık/koyu temaya göre
-- değişir. Yüklenmiş eski logo (koyu yazılı PNG) bunu ezdiği için temizlenir; dosya
-- uploads/branding altında kalır, Yönetim > Marka'dan yeniden seçilebilir.
UPDATE site_info SET content = (content::jsonb || '{"logo": "", "mobile_logo": ""}'::jsonb)::text
WHERE type = 'branding';
-- Rozet ikonları: "İstihbaratçı" (scholar) ve "Bilirkişi" (autobiographer). Adlar
-- çeviri dosyasından gelir; ikon tabloda durur, bu yüzden burada da güncellenir.
UPDATE badge SET icon = 'incognito' WHERE name = 'badge.default_badges.scholar.name';
UPDATE badge SET icon = 'mortarboard-fill' WHERE name = 'badge.default_badges.autobiographer.name';
-- id sıraları: kurulumun elle verdiği id'ler (yönetici = 1) sırayı ilerletmez; yeni
-- kurulmuş bir veritabanında ilk LDAP kullanıcısı "duplicate key ... user_pkey" ile
-- düşer. Sıra tablodaki en büyük id'ye çekilir, hiçbir zaman geri alınmaz.
-- (internal/migrations/init.go SyncIDSequencesSQL ile aynı.)
DO $$
DECLARE r record; m bigint;
BEGIN
  FOR r IN
    SELECT c.table_name AS t, pg_get_serial_sequence(quote_ident(c.table_name), 'id') AS s
    FROM information_schema.columns c
    WHERE c.table_schema = current_schema() AND c.column_name = 'id'
  LOOP
    IF r.s IS NOT NULL THEN
      EXECUTE format('SELECT max(id) FROM %I', r.t) INTO m;
      IF m IS NOT NULL THEN
        EXECUTE format('SELECT setval(%L, greatest(%s, last_value)) FROM %s', r.s, m, r.s);
      END IF;
    END IF;
  END LOOP;
END $$;

COMMIT;
