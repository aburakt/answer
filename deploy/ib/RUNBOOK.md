# Answer 1.6.0 → 2.0.3 geçiş runbook'u (intranet)

Kurulum: Answer Docker'da, PostgreSQL sunucunun kendisinde. Giriş Keycloak'tan
YAPI'deki SSO modülüne geçer (kaynak giris reposu `sso/`, belge: giris `/ldap-config`).

## Provada denenenler (17 Kasım 2026)

Yerel PostgreSQL 16 üzerinde, 1.6.0 kurulumu + Keycloak biçiminde üç kullanıcı
(iki `sub` UUID'li, biri zaten sicilli) ile:

| Adım | Sonuç |
| --- | --- |
| 1.6.0 kurulumu, `version` = 28 | tamam |
| `pg_dump -Fc` yedeği | tamam |
| `answer upgrade` (2.0.3) | 28 → 36, 8 migration, hata yok, 0,1 sn |
| `site_info` diff'i | ayarlar yeni anahtarlara bölündü; eklenenler aşağıda |
| `sql/10-keycloak-to-giris.sql` | 2 satır sicile çevrildi, 1 satır zaten sicil; ikinci çalıştırmada değişiklik yok |
| `sql/20-site-settings.sql` | e-posta doğrulama zorunluluğu kapandı, avatar `system` |
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
6. **Kimlik eşleme.** `psql -v ON_ERROR_STOP=1 -U answer -f sql/10-keycloak-to-giris.sql`
   İlk iki sorgu satır döndürürse dur: o kullanıcılar elle çözülür.
7. **Ayarlar.** `psql -v ON_ERROR_STOP=1 -U answer -f sql/20-site-settings.sql`
8. **Başlat.** `docker compose up -d`
9. **Connector'ı köprüye çevir.** Admin > Eklentiler > OAuth2 Temel: adresler
   giris `/ldap-config` sayfasındaki Answer tarifine göre, `User ID JSON path = sub`.
   Bir kullanıcıyla giriş yapıp kendi eski hesabına (itibarı, soruları) düştüğü doğrulanır.
10. **Şifreli girişi kapat.** Ancak 9. adım doğrulandıktan sonra: Admin > Giriş >
    şifreli giriş ve yeni kayıt kapalı.
11. **Hook kaynakları.** Admin > Eklentiler > Hook geçidi açılır, kaynaklar girilir,
    `ANSWER_HOOK_SECRET_<ID>` değerleri `.env`'e konur, `docker compose up -d`.

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

`script/plugin_list` connector-basic, reviewer-basic, captcha-basic, quick-links ve
`plugins/ib-hook`'u içerir. Build internet ister (Go ve npm modülleri); bu yüzden
VPS'te yapılır, sunucuya yalnız imaj gider.

## Yeni upstream sürümüne geçiş

Fork'taki değişiklikler küçük ve ayrı dosyalarda tutulur:

- `plugin/content_writer.go`, `internal/service/importer/content_func.go`
  (yeni dosyalar) ve `plugin_common_service.go`'da iki satır.
- `i18n/tr_TR.yaml` ve `i18n/tr_TR_test.go`: test, upstream'in eklediği her
  çevrilmemiş anahtarda kırılır.
- `plugins/ib-hook/` (ayrı Go modülü), `deploy/ib/`, `script/plugin_list`'te bir satır.

Yol: yeni etiketten `ib/vX.Y.Z` dalı açılır, bu commit'ler cherry-pick edilir,
`go test ./i18n/ ./plugins/ib-hook/...` yeşil olana kadar çeviri tamamlanır.
