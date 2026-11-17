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
-- Upgrade, v2.0.3'te `require_email_verification=true` ekliyor; giris'ten gelen
-- e-postalar zaten doğrulanmış sayıldığı için kapatılır. Varsayılan avatar
-- Gravatar'dı (intranette erişilemez): sistem avatarına çekilir. Fotoğraf
-- YAPI SSO'nun `picture` claim'inden gelir.
-- Şifreli giriş burada KAPATILMAZ: önce giris ile giriş doğrulanmalı (RUNBOOK).
BEGIN;
UPDATE site_info SET content = jsonb_set(content::jsonb, '{require_email_verification}', 'false')::text
WHERE type = 'login';
UPDATE site_info SET content = jsonb_set(content::jsonb, '{default_avatar}', '"system"')::text
WHERE type = 'users_settings';
COMMIT;
