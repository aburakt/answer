# Answer 1.6.0 → 2.0.3 geçiş runbook'u (intranet)

Kurulum: Answer Docker'da, PostgreSQL sunucunun kendisinde. Giriş Keycloak'tan
intranet LDAP'ına geçer: `plugins/ib-ldap` eklentisi, Answer'ın kendi login
ekranında sicil ve şifre sorar, doğrudan LDAP'a doğrulatır.

## Provada denenenler (17 Kasım 2026)

Yerel PostgreSQL 16 üzerinde, 1.6.0 kurulumu + Keycloak biçiminde üç kullanıcı
(iki `sub` UUID'li, biri zaten sicilli) ile:

| Adım | Sonuç |
| --- | --- |
| 1.6.0 kurulumu, `version` = 28 | tamam |
| `pg_dump -Fc` yedeği | tamam |
| `answer upgrade` (2.0.3) | 28 → 36, 8 migration, hata yok, 0,1 sn |
| `site_info` diff'i | ayarlar yeni anahtarlara bölündü; eklenenler aşağıda |
| `sql/10-keycloak-to-ldap.sql` | 3 Keycloak hesabına sicilli `ldap` satırı eklendi; ikinci çalıştırmada yeni satır yok (PostgreSQL 16, 18 Kasım) |
| `sql/20-site-settings.sql` | e-posta doğrulama zorunluluğu kapandı, avatar `system`, kayıt kapandı, ana renk lacivert (23 Kasım), id sıraları düzeltildi, üst bar beyaz, yüklenmiş logo temizlendi (yerleşik amblem), iki rozet ikonu (24 Kasım) |
| LDAP girişi (24 Kasım) | gerçek build + nginx (TLS) + PostgreSQL 16 + Samba Active Directory ile uçtan uca: ldaps, StartTLS, şifresiz, IP adresi, iç CA, hatalı ayarlar, e-postasız ve pasif hesaplar |
| `answer run` + `/answer/api/v1/siteinfo` | 200 |
| `pg_restore` ile geri dönüş | `version` = 28, eski kayıtlar geri geldi |

Upgrade'in eklediği ve kontrol edilmesi gereken ayarlar: `questions.min_tags=1`,
`questions.min_content=6`, `questions.restrict_answer=true`,
`login.require_email_verification=true` (20-site-settings kapatır),
`ai.enabled=false`, `security.check_update=false`.

Gerçek veriyle prova dev VPS'te prod DB kopyasıyla yapılmalı (aşağıdaki sıra).

## Sıra

Konteynerin entrypoint'i her açılışta `answer upgrade` çalıştırır. Bu yüzden
yeni imaj `docker compose up` ile değil, adım adım başlatılır.

1. **Envanter.** Mevcut konteynerin `/data/conf/config.yaml`'ı (DSN), plugin
   listesi (`docker exec <eski> answer plugin`) ve connector ayarı:
   `SELECT value FROM plugin_config WHERE plugin_slug_name = 'basic_connector';`
   (`user_id_json_path` neydi?).
2. **Durdur ve yedekle.**
   ```bash
   docker stop answer
   pg_dump -Fc -U answer answer > answer-$(date +%F).dump
   docker run --rm -v answer-data:/data -v "$PWD":/b alpine tar czf /b/answer-data.tgz /data
   psql -U answer -Atc "select type, content from site_info order by type" > site_info_once.txt
   ```
3. **İmajı yükle.** `docker load < ib-answer-2.0.3-ib.1.tar.gz`
4. **Upgrade.**
   ```bash
   docker run --rm --add-host host.docker.internal:host-gateway \
     -v answer-data:/data --entrypoint answer ib/answer:2.0.3-ib.1 upgrade -C /data
   ```
5. **Ayarları kontrol et.** `site_info` yeniden alınıp 2. adımdaki dökümle karşılaştırılır.
6. **Kimlik eşleme.** `psql -v ON_ERROR_STOP=1 -U answer -f sql/10-keycloak-to-ldap.sql`
   İlk iki sorgu satır döndürürse dur: o kullanıcılar elle çözülür.
7. **Ayarlar.** `psql -v ON_ERROR_STOP=1 -U answer -f sql/20-site-settings.sql`
   Rozet ikonlarını uygulama her açılışta kendisi de düzeltir; betiğin geri kalanı
   (kayıt kapatma, tema, varsayılan avatar) yalnız bu betikle gelir.
8. **Başlat.** `docker compose up -d`
9. **LDAP connector'ı aç.** Admin > Eklentiler > LDAP connector:
   - `LDAP sunucusu`: `ldaps://<sunucu>:636` ya da `ldap://<sunucu>:389`.
   - `Bağlantı güvenliği`: `ldap://` adresinde StartTLS denenir. Sunucuda TLS
     yoksa "Şifresiz" seçilir.
   - `Sertifika doğrulaması`: sunucu IP adresiyle yazıldıysa ya da sertifikası iç
     CA'dan geliyorsa "doğrulama" açılır, ya da CA'nın PEM dosyası `/data` altına
     konup `CA sertifika dosyası`na yolu yazılır.
   - `Base DN`, `Bind DN` ve `Bind parolası`: salt okunur bir servis hesabı. Bind DN
     `kullanici@alan` biçiminde de yazılabilir. Parola yalnız admin ekranına
     girilir, hiçbir dosyaya yazılmaz.
   - `Kullanıcı alanı` ve `Dış kimlik alanı`: sicili veren alan (`sAMAccountName`).
   - `E-posta alan adı`: `egmidb.gov.tr`. LDAP kaydında `mail` boşsa e-posta
     `sicil@egmidb.gov.tr` olur; boş bırakılırsa e-postası olmayan giremez.
   - `Deneme sicili` yazılıp **Kaydet**'e, sonra **Bağlantıyı dene**'ye basılır.
     Deneme bağlantıyı, servis hesabını ve o sicilin kaydını sınar (kullanıcı
     parolası gerekmez) ve sonucu ya da hatanın nedenini ekranda gösterir.
   Sonra bir kullanıcıyla giriş yapılır; kendi eski hesabına (itibarı, soruları)
   düştüğü doğrulanır.
10. **Keycloak'ı kapat.** 9. adım doğrulandıktan sonra Admin > Eklentiler > OAuth2
    Temel kapatılır. Şifreli giriş **açık kalır**: yalnız yöneticiler içindir ve
    `/users/admin` adresinden yapılır; yönetici olmayan hesap şifreyle giremez.
    Kullanıcılar `/users/login`'de doğrudan sicil ve parola (LDAP) formunu görür,
    kayıt bağlantısı yoktur. LDAP'tan ilk kez gelen kullanıcının hesabı ilk girişte
    kendiliğinden açılır (kayıt kapalı olsa da).
11. **Dil.** Admin > Arayüz > Dil `Türkçe` seçilir. Tarayıcısı Türkçe olanlar
    zaten Türkçe görür; bu, varsayılanı da Türkçe yapar.
12. **Bot hesabı.** Kılavuz konularını açan hesap (ör. `kilavuz-bot@egmidb.gov.tr`)
    Admin > Kullanıcılar'dan yerel hesap olarak açılır; LDAP'ta olması gerekmez,
    login olmaz.
    Hook kaynağındaki `bot_email` bu hesabın e-postasıyla aynı olmalı; hesap yoksa
    plugin 422 döner ve Kılavuz o olayı yeniden denemez.
13. **Hook kaynakları.** Admin > Eklentiler > Hook geçidi açılır, kaynaklar girilir,
    `ANSWER_HOOK_SECRET_<ID>` değerleri `.env`'e konur, `docker compose up -d`.

## LDAP girişinde dikkat

- Form `/answer/api/v1/connector/redirect/ldap`'a POST eder (fork bu route'u ekler).
  İstek sitenin kendisinden gelmeli: `Origin`, Admin > Genel'deki site adresiyle ya
  da isteğin `Host` başlığıyla aynı host'ta olmalı. nginx `proxy_set_header Host
  $host;` ile `Host`'u korumalı. Başarılı girişten sonra kullanıcı site adresine
  yönlendirilir; site adresi `https://forum.egmidb.gov.tr` olmalı.
- Giriş düşerse form nedeni söyler ve `docker logs answer 2>&1 | grep -i ldap`
  ayrıntıyı verir (parolalar log'a yazılmaz):

  | Kod | Anlamı | Bakılacak yer |
  |---|---|---|
  | (kodsuz) | Sicil LDAP'ta yok ya da parola yanlış | Kullanıcı alanı, Base DN |
  | `connect` | Sunucuya ulaşılamadı (5 sn) | adres, port, konteynerden DNS, güvenlik duvarı |
  | `tls` | TLS kurulamadı | Bağlantı güvenliği, Sertifika doğrulaması, CA dosyası |
  | `bind` | Servis hesabı reddedildi | Bind DN, Bind parolası |
  | `search` | Arama hatası | Base DN, servis hesabının okuma yetkisi, çift sicil |
  | `mail` | Kayıtta `mail` yok | E-posta alan adı |
  | `identity` | Dış kimlik alanı boş | Dış kimlik alanı |
  | `config` | Zorunlu ayar boş ya da adres hatalı | eklenti ayarları |
  | `origin` | İstek başka adresten geldi | site adresi, nginx `Host` |
  | `account` | LDAP'taki e-posta başka bir hesapta ya da hesap kapalı | `sql/10`, Admin > Kullanıcılar |
  | `internal` | Hesap oluşturulamadı | `docker logs`, veritabanı |

- `account`: Answer, tanımadığı bir sicil forumda zaten kayıtlı bir e-postayla
  gelirse girişi reddeder. Keycloak'tan gelen hesapları `sql/10` sicille eşler;
  eşlenmemiş hesap için o kullanıcıya `user_external_login`'de `provider='ldap'`,
  `external_id=<sicil>` satırı eklenir.
- Konteyner LDAP sunucusunun adını çözemiyorsa compose'a `extra_hosts` ya da `dns`
  eklenir, ya da adres IP ile yazılıp sertifika doğrulaması kapatılır.
- Gravatar intranetten erişilemediği için kapalıdır: seçenek arayüzden kaldırıldı,
  veritabanında kalmış bir gravatar seçimi de `system` avatar olarak gösterilir.
- Kullanıcı adı LDAP kullanıcı adıdır (sicil): ilk girişte öyle açılır, LDAP'ta
  değişirse sonraki girişte güncellenir. Görünen ad da LDAP'taki addır ve her
  girişte eşitlenir. Yönetici dışındaki kullanıcı kendi kullanıcı adını, görünen
  adını ve profil resmini değiştiremez (sunucu isteği yok sayar).
- Profil resmi: Yönetim > Kullanıcılar > Ayarlar'daki "Profil resmi servisi
  adresi" doluysa her girişte `GET <adres>/<sicil>` ile alınır (kimlik bilgisi
  gönderilmez). Yanıt ham resim, base64 metin ya da içinde base64 resim olan JSON
  olabilir. Resim eklentinin deposunda saklanır ve eski sürümden gelen profil
  resminin yerine geçer. Resim alınamazsa giriş yine başarılıdır, eski resim
  kalır; neden konteyner log'unda "profil resmi alınamadı" ya da "resim
  döndürmedi" satırındadır. Adres boşsa ya da kurulumdan kalan gravatar.com
  adresiyse hiçbir istek yapılmaz. Sertifika ayarları LDAP eklentisindeki
  CA/doğrulama ayarlarıdır.
- Geri dönüş: OAuth2 Temel yeniden açılır, `basic` satırları yerinde duruyor.

## Kilitlenirsen

```bash
docker exec -it answer answer config -c allow_password_login -C /data
docker exec -it answer answer passwd -e admin@egmidb.gov.tr -C /data
docker restart answer
```

Yerel şifreli bir admin hesabı her zaman tutulur.

## Geri dönüş

Migration'lar transaction içinde değil; geri dönüş yalnız yedekten:

```bash
docker compose down
dropdb -U postgres answer && createdb -U postgres -O answer answer
pg_restore -U answer -d answer --no-owner answer-YYYY-MM-DD.dump
# answer-data.tgz geri açılır, eski 1.6.0 imajı başlatılır
```

## İmajı hazırlama (dev VPS)

```bash
docker compose -f deploy/ib/docker-compose.yaml build
docker save ib/answer:2.0.3-ib.1 | gzip > ib-answer-2.0.3-ib.1.tar.gz
```

`script/plugin_list` connector-basic, reviewer-basic, captcha-basic, quick-links ile repodaki
`plugins/ib-ldap` ve `plugins/ib-hook`'u içerir. Build internet ister (Go ve npm modülleri); bu yüzden
VPS'te yapılır, sunucuya yalnız imaj gider.

## Yeni upstream sürümüne geçiş

Fork'taki değişiklikler küçük ve ayrı dosyalarda tutulur:

- `plugin/content_writer.go`, `internal/service/importer/content_func.go`
  (yeni dosyalar) ve `plugin_common_service.go`'da iki satır.
- `i18n/tr_TR.yaml` ve `i18n/tr_TR_test.go`: test, upstream'in eklediği her
  çevrilmemiş anahtarda kırılır. `i18n/i18n.yaml`'a Türkçe dil seçeneği eklidir
  (upstream listede yok; yoksa Türkçe hiç sunulmaz, test bunu da denetler).
- Giriş: `ui/src/pages/Users/IbLogin` (LDAP formu, `/users/login`),
  `/users/admin` (yönetici şifre girişi), `EmailLogin`'de yönetici kontrolü,
  `plugin_api_router.go`'da LDAP POST route'u.
- Tema: `ui/src/common/_ib-variables.scss` ve `ib-theme.scss` (lacivert/altın),
  arayüz metinleri `// ib:` yorumuyla işaretli. Üst bar ve logo
  `components/Header/index.tsx`, footer `components/Footer` ve
  `ui/template/footer.html`, favicon `ui/public/favicon.ico`.
- Rozet ikonları `internal/migrations/init_data.go` `defaultBadgeTable`'da;
  upstream bu tabloyu değiştirirse iki `// ib:` satırı korunur.
- Profil kilidi: `user_service.go` `UpdateInfo`/`formatUserInfoForUpdateInfo`,
  `user_external_login_service.go` `syncUsername` ve avatar eşitlemesi,
  `Users/Settings/Profile`.
- `plugins/ib-hook/` ve `plugins/ib-ldap/` (ayrı Go modülleri), `deploy/ib/`,
  `script/plugin_list`'te iki satır. `ib-ldap`, upstream `connector-ldap`'tan türedi
  ve onun yerine kullanılır (upstream şifresiz LDAP'ı, IP adresini ve iç CA'yı
  dosyasız kabul etmiyor, hatanın nedenini de vermiyordu).
- `internal/migrations/init.go` `SyncIDSequencesSQL`: yeni PostgreSQL kurulumunda
  id sıralarını düzeltir.

Yol: yeni etiketten `ib/vX.Y.Z` dalı açılır, bu commit'ler cherry-pick edilir,
`go test ./i18n/ ./plugins/ib-hook/...` yeşil olana kadar çeviri tamamlanır.

## Duyuru

Ayrı bir duyuru işlemi yoktur: kapatılmış soru duyuru sayılır. Yönetici ya da
moderatör soruyu **Kapat** ile kapatır (istenirse **Sabitle** ile üste alır);
soru listelerde `[DUYURU]` ekiyle, sayfasında "DUYURU / Duyuru tarihi" bandıyla
görünür. Bu yalnız çeviri ve arayüzdür (`i18n/tr_TR.yaml` içindeki `closed`
anahtarları, `ui/src/pages/Questions/Detail/components/IbAnnouncement`);
veritabanında değişiklik yoktur, eski sürümde kapatılmış sorular kendiliğinden
duyuru olarak görünür.
