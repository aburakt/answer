# DevOps için not: Answer 2.0.3 geçişi

Merhaba. Intranetteki Answer'ı 1.6.0'dan 2.0.3'e taşıyoruz. Giriş Keycloak'tan
giris'e geçiyor. Geçişin adımları `RUNBOOK.md`'de. 1.6.0'ı sen kurduğun için
aşağıdakiler yalnız sende var. Soru olursa Burak'a yaz.

**Sır göndermeyin.** Parola, client secret ya da DSN'deki parola gibi değerler bu
nota yazılmaz. Yalnız adları ve sunucuda nerede durdukları lazım.

## 1. Senden istediklerimiz

### 1.1 Kurulumun kendisi

- [ ] Kullanılan imaj ve etiketi: `docker inspect <konteyner> --format '{{.Config.Image}}'`
- [ ] İmajı sen mi build ettin? Ettiysen Dockerfile'ı ve değiştirdiğin dosyalar
      (diff ya da dosyanın kendisi).
- [ ] `docker run` komutu ya da compose dosyası. Port, volume, env ve
      `extra_hosts` bize lazım.
- [ ] `/data/conf/config.yaml`. Parolayı `***` yapıp gönderin.
- [ ] Önünde nginx varsa ilgili `server`/`location` bloğu ve dış adres
      (ör. `https://answer.egmidb.gov.tr`). Yol kökte mi, `/answer` altında mı?
- [ ] Yüklü plugin'ler: `docker exec <konteyner> answer plugin`
- [ ] Şema sürümü: `SELECT * FROM version;` (provada 1.6.0'ı 28 olarak gördük).

### 1.2 Keycloak bağlantısı

- [ ] Connector ayarı. Secret alanını silip gönderin:
      `SELECT plugin_slug_name, value FROM plugin_config;`
      Özellikle `user_id_json_path` neydi? `sub` ise kimlik eşleme SQL'i
      (`sql/10-keycloak-to-giris.sql`) gerekiyor, `preferred_username` ise
      gerekmiyor.
- [ ] Bir örnek satır. Sicil maskelenebilir:
      `SELECT provider, external_id, left(meta_info, 300) FROM user_external_login LIMIT 3;`
- [ ] Keycloak dışında şifreyle giren hesap var mı (ör. admin)? Geçişten sonra
      şifreli giriş kapanacak. Admin'e acil durumda nasıl girileceği
      RUNBOOK'ta "Kilitlenirsen" bölümünde yazıyor.

### 1.3 1.6.0'a yaptığın özelleştirmeler

Hangisini nasıl yaptığını yaz: admin ayarı, custom CSS/HTML ya da kod
değişikliği. Kod değiştiysen dosyanın kendisini de gönder.

- [ ] **Footer** (giris'teki footer): içerik ve stil.
- [ ] **Yatay uzun logo**: dosyanın kendisi (svg/png) ve nasıl yüklendiği.
- [ ] **"Soru oluştur" butonu çevirisi**: hangi dosyada neyi değiştirdin? 2.0.3'te
      bunu `i18n/tr_TR.yaml` içinde düzelttik. Bizim düzeltmemizle aynı yeri mi
      değiştirmişsin, bunu kontrol edeceğiz.
- [ ] **Profil fotoğrafı** (Gravatar yerine Redis'ten):
      - Nasıl yapıldı? Örneğin bir plugin, nginx kuralı, kod değişikliği ya da ayrı bir servis.
      - Hangi Redis'e bağlanıyor (host/port)? Parolanın yalnız yerini yaz.
      - Anahtar biçimi ne?
      - YAPI `get_foto_from_redis_by_sicil` mi çağrılıyor?
      Yeni düzende fotoğraf giriş anında giris-sso tarafından alınıyor. Eskisini
      kaldırabilir miyiz, bunu bilmemiz gerekiyor.
- [ ] Bunların dışında değiştirdiğin bir şey varsa o da.

### 1.4 Sunucu

- [ ] PostgreSQL sürümü (`psql --version`) ve Answer'ın veritabanı adı.
- [ ] PostgreSQL konteynerden nasıl erişiliyor? `host.docker.internal`,
      sunucu IP'si ya da başka bir yol? `pg_hba.conf`'taki ilgili satır.
- [ ] Docker ve Compose sürümü (`docker version`, `docker compose version`).
- [ ] `/data` volume'ünün boyutu (`du -sh`). Yedek için bilmemiz gerekiyor.
- [ ] Prova için prod DB'nin bir kopyasını dev VPS'e taşıyabilir miyiz?
      Taşınamıyorsa provayı sunucuda ayrı bir veritabanında yaparız.

## 2. Bizim tarafımızda hazır olanlar

- Imaj: `ib/answer:2.0.3-ib.1`. Dev VPS'te build edilir, `docker save` ile USB'ye
  alınır, sunucuda `docker load` edilir.
- `docker-compose.yaml` ve `.env.example`: PostgreSQL host'ta kalır,
  `host-gateway` ile erişilir.
- `sql/10-keycloak-to-giris.sql` ve `sql/20-site-settings.sql`. İkisi de iki kez
  çalıştırılırsa sorun çıkarmaz.
- `RUNBOOK.md`: yedek, upgrade, eşleme, smoke test ve geri dönüş adımları.
- giris-sso (daire-devs/giris, `sso/`): ayrı bir küçük servis olarak kurulacak.
  Kurulumu ayrıca yazacağız.

## 3. Geçiş günü senden beklenen

Tarihi birlikte belirleyelim. Tahmini kesinti 15-30 dakika: yedek, upgrade ve
kontrol. Prod'daki her adım Burak'ın onayıyla yapılacak.
