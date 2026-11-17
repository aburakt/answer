-- ib: v2.0.3 upgrade'inden sonra intranet ayarları.
-- Upgrade, v2.0.3'te `require_email_verification=true` ekliyor; giris'ten gelen
-- e-postalar zaten doğrulanmış sayıldığı için kapatılır. Varsayılan avatar
-- Gravatar'dı (intranette erişilemez): sistem avatarına çekilir. Fotoğraf
-- giris-sso'nun `picture` claim'inden gelir.
-- Şifreli giriş burada KAPATILMAZ: önce giris ile giriş doğrulanmalı (RUNBOOK).
BEGIN;
UPDATE site_info SET content = jsonb_set(content::jsonb, '{require_email_verification}', 'false')::text
WHERE type = 'login';
UPDATE site_info SET content = jsonb_set(content::jsonb, '{default_avatar}', '"system"')::text
WHERE type = 'users_settings';
COMMIT;
