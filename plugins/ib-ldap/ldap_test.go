/*
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package ibldap

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/md5" //nolint:gosec // sözleşme
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/apache/answer/plugin"
	"github.com/gin-gonic/gin"
	goldap "github.com/go-ldap/ldap/v3"
	"github.com/jimlambrt/gldap"
)

const (
	svcDN, svcPass = "CN=svc-forum,OU=Personel,DC=ib,DC=test", "servis-parolasi"
	receiver       = "https://forum.ib.test/answer/api/v1/connector/redirect/ldap"
)

type person struct{ dn, pass, mail string }

// Sahte dizin. sAMAccountName -> kayıt; "cift" iki kayıt döndürür.
var people = map[string]person{
	"1234567": {"CN=Çağrı Öztürk,OU=Personel,DC=ib,DC=test", "dogru", "Cagri.Ozturk@egmidb.gov.tr"},
	"7654321": {"CN=Mailsiz Kişi,OU=Personel,DC=ib,DC=test", "dogru", ""},
}

// startDirectory sahte LDAP sunucusunu açar ve ldap:// ya da ldaps:// adresini döner.
// CA dosyasının yolu yalnız TLS'li sunucuda doludur.
func startDirectory(t *testing.T, withTLS bool) (server, caPath string) {
	t.Helper()
	mux, err := gldap.NewMux()
	if err != nil {
		t.Fatal(err)
	}
	_ = mux.Bind(func(w *gldap.ResponseWriter, r *gldap.Request) {
		resp := r.NewBindResponse(gldap.WithResponseCode(gldap.ResultInvalidCredentials))
		defer func() { _ = w.Write(resp) }()
		m, err := r.GetSimpleBindMessage()
		if err != nil {
			return
		}
		ok := m.UserName == svcDN && string(m.Password) == svcPass
		for _, p := range people {
			ok = ok || (m.UserName == p.dn && string(m.Password) == p.pass)
		}
		// Gerçek sunucular boş parolayı anonim bind sayıp kabul eder.
		if ok || m.Password == "" {
			resp.SetResultCode(gldap.ResultSuccess)
		}
	})
	_ = mux.Search(func(w *gldap.ResponseWriter, r *gldap.Request) {
		resp := r.NewSearchDoneResponse(gldap.WithResponseCode(gldap.ResultNoSuchObject))
		defer func() { _ = w.Write(resp) }()
		m, err := r.GetSearchMessage()
		if err != nil || !strings.HasSuffix(strings.ToLower(m.BaseDN), "dc=ib,dc=test") {
			return
		}
		sicil := strings.TrimSuffix(strings.TrimPrefix(m.Filter, "(sAMAccountName="), ")")
		entry := func(dn, sicil, mail string) {
			attrs := map[string][]string{"sAMAccountName": {sicil}, "displayName": {strings.TrimPrefix(strings.Split(dn, ",")[0], "CN=")}}
			if mail != "" {
				attrs["mail"] = []string{mail}
			}
			_ = w.Write(r.NewSearchResponseEntry(dn, gldap.WithAttributes(attrs)))
		}
		if sicil == "cift" {
			entry("CN=A,DC=ib,DC=test", "cift", "a@x")
			entry("CN=B,DC=ib,DC=test", "cift", "b@x")
		} else if p, ok := people[sicil]; ok {
			entry(p.dn, sicil, p.mail)
		}
		resp.SetResultCode(gldap.ResultSuccess)
	})
	srv, err := gldap.NewServer()
	if err != nil {
		t.Fatal(err)
	}
	_ = srv.Router(mux)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	opts, scheme := []gldap.Option{}, "ldap://"
	if withTLS {
		cert, ca := selfSigned(t)
		caPath = ca
		opts, scheme = append(opts, gldap.WithTLSConfig(&tls.Config{Certificates: []tls.Certificate{cert}})), "ldaps://"
	}
	go func() { _ = srv.Run(addr, opts...) }()
	t.Cleanup(func() { _ = srv.Stop() })
	for i := 0; i < 100; i++ {
		if c, err := net.Dial("tcp", addr); err == nil {
			_ = c.Close()
			return scheme + addr, caPath
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("sahte LDAP açılmadı")
	return
}

// selfSigned 127.0.0.1 için kendinden imzalı sertifika üretir ve PEM'ini dosyaya yazar.
func selfSigned(t *testing.T) (tls.Certificate, string) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, path
}

func newConnector(t *testing.T, cfg Config) *Connector {
	t.Helper()
	if cfg.BaseDN == "" {
		cfg.BaseDN = "DC=ib,DC=test"
	}
	if cfg.BindDN == "" {
		cfg.BindDN, cfg.BindPassword = svcDN, svcPass
	}
	raw, _ := json.Marshal(cfg)
	c := New()
	if err := c.ConfigReceiver(raw); err != nil {
		t.Fatal(err)
	}
	return c
}

func loginRequest(username, password string) *http.Request {
	form := url.Values{"username": {username}, "password": {password}}
	r := httptest.NewRequest(http.MethodPost, "http://forum.ib.test/answer/api/v1/connector/redirect/ldap",
		strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://forum.ib.test")
	return r
}

func codeOf(err error) string {
	var coded interface{ LoginErrorCode() string }
	if errors.As(err, &coded) {
		return coded.LoginErrorCode()
	}
	if err != nil {
		return "kodsuz: " + err.Error()
	}
	return ""
}

func TestLogin(t *testing.T) {
	plain, _ := startDirectory(t, false)
	secure, ca := startDirectory(t, true)
	closed := func() string {
		l, _ := net.Listen("tcp", "127.0.0.1:0")
		defer l.Close()
		return "ldap://" + l.Addr().String()
	}()

	cases := []struct {
		name       string
		cfg        Config
		user, pass string
		wantCode   string
		wantMail   string
	}{
		{name: "şifresiz ldap", cfg: Config{Server: plain, Security: "none"}, user: "1234567", pass: "dogru", wantMail: "cagri.ozturk@egmidb.gov.tr"},
		{name: "şemasız adres ldap sayılır", cfg: Config{Server: strings.TrimPrefix(plain, "ldap://"), Security: "none"}, user: "1234567", pass: "dogru", wantMail: "cagri.ozturk@egmidb.gov.tr"},
		{name: "sicilin çevresindeki boşluk", cfg: Config{Server: plain, Security: "none"}, user: " 1234567 ", pass: "dogru", wantMail: "cagri.ozturk@egmidb.gov.tr"},
		{name: "yanlış parola", cfg: Config{Server: plain, Security: "none"}, user: "1234567", pass: "yanlis", wantCode: CodeCredentials},
		{name: "boş parola sunucuya gitmez", cfg: Config{Server: plain, Security: "none"}, user: "1234567", pass: "", wantCode: CodeCredentials},
		{name: "olmayan sicil", cfg: Config{Server: plain, Security: "none"}, user: "9999999", pass: "dogru", wantCode: CodeCredentials},
		{name: "filtre enjeksiyonu", cfg: Config{Server: plain, Security: "none"}, user: "*", pass: "dogru", wantCode: CodeCredentials},
		{name: "çift kayıt", cfg: Config{Server: plain, Security: "none"}, user: "cift", pass: "dogru", wantCode: CodeSearch},
		{name: "yanlış Base DN", cfg: Config{Server: plain, Security: "none", BaseDN: "DC=yok"}, user: "1234567", pass: "dogru", wantCode: CodeSearch},
		{name: "servis parolası yanlış", cfg: Config{Server: plain, Security: "none", BindDN: svcDN, BindPassword: "x"}, user: "1234567", pass: "dogru", wantCode: CodeBind},
		{name: "ayar eksik", cfg: Config{Security: "none"}, user: "1234567", pass: "dogru", wantCode: CodeConfig},
		{name: "geçersiz şema", cfg: Config{Server: "http://x"}, user: "1234567", pass: "dogru", wantCode: CodeConfig},
		{name: "mailsiz, alan adı yok", cfg: Config{Server: plain, Security: "none"}, user: "7654321", pass: "dogru", wantCode: CodeMail},
		{name: "mailsiz, alan adıyla", cfg: Config{Server: plain, Security: "none", MailDomain: "@egmidb.gov.tr"}, user: "7654321", pass: "dogru", wantMail: "7654321@egmidb.gov.tr"},
		{name: "kapalı port", cfg: Config{Server: closed, Security: "none"}, user: "1234567", pass: "dogru", wantCode: CodeConnect},
		{name: "TLS'siz sunucuda StartTLS", cfg: Config{Server: plain}, user: "1234567", pass: "dogru", wantCode: CodeTLS},
		{name: "ldaps, tanınmayan sertifika", cfg: Config{Server: secure}, user: "1234567", pass: "dogru", wantCode: CodeTLS},
		{name: "ldaps, doğrulama kapalı", cfg: Config{Server: secure, TLSSkipVerify: true}, user: "1234567", pass: "dogru", wantMail: "cagri.ozturk@egmidb.gov.tr"},
		{name: "ldaps, CA dosyasıyla", cfg: Config{Server: secure, TLSCACertPath: ca}, user: "1234567", pass: "dogru", wantMail: "cagri.ozturk@egmidb.gov.tr"},
		{name: "ldaps, CA dosyası yok", cfg: Config{Server: secure, TLSCACertPath: "/yok/ca.pem"}, user: "1234567", pass: "dogru", wantCode: CodeTLS},
		{name: "kayıtlı entryUUID AD'de sicile döner", cfg: Config{Server: plain, Security: "none", UserAttr: "sAMAccountName", ExternalIDAttr: "entryUUID"}, user: "1234567", pass: "dogru", wantMail: "cagri.ozturk@egmidb.gov.tr"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info, err := newConnector(t, tc.cfg).login(loginRequest(tc.user, tc.pass), receiver, photoCaller{})
			if got := codeOf(err); got != tc.wantCode {
				t.Fatalf("kod = %q (%v), beklenen %q", got, err, tc.wantCode)
			}
			if err != nil {
				if strings.Contains(err.Error(), svcPass) || (tc.pass != "" && strings.Contains(err.Error(), tc.pass)) {
					t.Fatalf("hata metni parola içeriyor: %v", err)
				}
				return
			}
			if info.Email != tc.wantMail || info.ExternalID != strings.TrimSpace(tc.user) || info.Username != strings.TrimSpace(tc.user) {
				t.Fatalf("beklenmeyen kullanıcı: %+v", info)
			}
			if tc.user == "1234567" && info.DisplayName != "Çağrı Öztürk" {
				t.Fatalf("ad = %q", info.DisplayName)
			}
		})
	}
}

func TestBindErrorClassifiesStrongAuth(t *testing.T) {
	strong := goldap.NewError(goldap.LDAPResultStrongAuthRequired, errors.New("BindSimple: Transport encryption required."))
	if got := bindError(Config{}, strong).Code; got != CodeTLS {
		t.Fatalf("kod = %q", got)
	}
	wrong := goldap.NewError(goldap.LDAPResultInvalidCredentials, errors.New("data 52e"))
	if got := bindError(Config{}, wrong).Code; got != CodeBind {
		t.Fatalf("kod = %q", got)
	}
}

func TestCheckSameOrigin(t *testing.T) {
	cases := []struct {
		name                          string
		origin, referer, host, fwHost string
		ok                            bool
	}{
		{name: "site adresi", origin: "https://forum.ib.test", host: "127.0.0.1:9080", ok: true},
		{name: "proxy şemayı düşürdü", origin: "http://forum.ib.test", host: "forum.ib.test", ok: true},
		{name: "site adresi yanlış ama Host aynı", origin: "https://10.0.0.5", host: "10.0.0.5", ok: true},
		{name: "X-Forwarded-Host", origin: "https://f.ib.test", host: "answer:80", fwHost: "f.ib.test", ok: true},
		{name: "yalnız Referer", referer: "https://forum.ib.test/users/login", host: "x", ok: true},
		{name: "Origin null, Referer var", origin: "null", referer: "https://forum.ib.test/users/login", host: "x", ok: true},
		{name: "başka site", origin: "https://kotu.example", host: "forum.ib.test"},
		{name: "başka site, Referer bizden", origin: "https://kotu.example", referer: "https://forum.ib.test/", host: "forum.ib.test"},
		{name: "başlık yok", host: "forum.ib.test"},
		{name: "alt alan adı", origin: "https://forum.ib.test.kotu.example", host: "forum.ib.test"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", nil)
			r.Host = tc.host
			for k, v := range map[string]string{"Origin": tc.origin, "Referer": tc.referer, "X-Forwarded-Host": tc.fwHost} {
				if v != "" {
					r.Header.Set(k, v)
				}
			}
			err := checkSameOrigin(r, receiver)
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, ok beklenen %v", err, tc.ok)
			}
			if err != nil && codeOf(err) != CodeOrigin {
				t.Fatalf("kod = %q", codeOf(err))
			}
		})
	}
}

func TestAdminTest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	plain, _ := startDirectory(t, false)
	call := func(cfg Config) (int, string) {
		c := newConnector(t, cfg)
		engine := gin.New()
		c.RegisterAuthAdminRouter(engine.Group("/answer/admin/api"))
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/answer/admin/api/ib-ldap/test", nil))
		return w.Code, w.Body.String()
	}
	if code, body := call(Config{Server: plain, Security: "none", TestUser: "1234567"}); code != http.StatusOK ||
		!strings.Contains(body, "cagri.ozturk@egmidb.gov.tr") {
		t.Fatalf("başarılı deneme: %d %s", code, body)
	}
	if code, body := call(Config{Server: plain, Security: "none"}); code != http.StatusOK {
		t.Fatalf("sicilsiz deneme: %d %s", code, body)
	}
	code, body := call(Config{Server: plain, Security: "none", BindDN: svcDN, BindPassword: "yanlis-parola"})
	if code != http.StatusBadRequest || !strings.Contains(body, "ib_ldap.bind") || strings.Contains(body, "yanlis-parola") {
		t.Fatalf("hatalı deneme: %d %s", code, body)
	}
}

func TestConfigFieldsKeepSecretsOutOfDefaults(t *testing.T) {
	c := newConnector(t, Config{Server: "dc1:389"})
	byName := map[string]any{}
	for _, f := range c.ConfigFields() {
		byName[f.Name] = f.Value
	}
	if byName["user_attr"] != "sAMAccountName" || byName["external_id_attr"] != "sAMAccountName" || byName["security"] != "tls" {
		t.Fatalf("varsayılanlar: %v", byName)
	}
	if byName["server"] != "dc1:389" {
		t.Fatalf("yazılan adres değiştirilmeden gösterilmeli: %v", byName["server"])
	}
}

type memStore map[string]string

func (m memStore) Get(_ context.Context, p plugin.KVParams) (string, error) {
	v, ok := m[p.Group+"/"+p.Key]
	if !ok {
		return "", plugin.ErrKVKeyNotFound
	}
	return v, nil
}

func (m memStore) Set(_ context.Context, p plugin.KVParams) error {
	m[p.Group+"/"+p.Key] = p.Value
	return nil
}

// fakeYAPI giris'in iki isteğini bekler: önce "giris" (X-Token: login), sonra
// get_foto_from_redis_by_sicil (X-Token: oturum token'ı, X-Uid: sicil).
func fakeYAPI(t *testing.T, photo []byte) (*httptest.Server, *[]http.Header) {
	t.Helper()
	var seen []http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		seen = append(seen, r.Header.Clone())
		switch {
		case body["maintask"] == "giris":
			if r.Header.Get("X-Token") != "login" || body["password"] != "dogru" || body["application"] != "GIRIS_DASHBOARD" {
				_, _ = w.Write([]byte(`{"login":false}`))
				return
			}
			_, _ = w.Write([]byte(`{"login":true,"X-Token":"tok-1"}`))
		case body["subtask"] == "get_foto_from_redis_by_sicil":
			if r.Header.Get("X-Token") != "tok-1" || r.Header.Get("X-Uid") != body["sicil"] ||
				body["application"] != "PERSONEL_WEB" || body["maintask"] != "personel" || body["uid"] != body["sicil"] {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write(photo)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

var pngBytes = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00")

func TestAvatarFromYAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	srv, seen := fakeYAPI(t, pngBytes)
	c := New()
	c.kv = memStore{}
	cfg := Config{PhotoURL: srv.URL}.normalized()
	caller := photoCaller{ClientIP: "10.0.0.7", UserAgent: "Mozilla/5.0"}

	path := c.avatarFor(context.Background(), cfg, "1234567", "dogru", caller)
	if !strings.HasPrefix(path, "/answer/api/v1/ib-ldap/foto/1234567-") {
		t.Fatalf("avatar yolu = %q", path)
	}
	// Başlıklar giris ile aynı: X-Clientip, X-User-Agent = md5(userAgent).
	for i, h := range *seen {
		if h.Get("X-Clientip") != "10.0.0.7" || h.Get("X-User-Agent") != md5hex("Mozilla/5.0") {
			t.Errorf("istek %d başlıkları = %v", i, h)
		}
	}
	if (*seen)[0].Get("X-Uid") != "" {
		t.Errorf("login isteğinde X-Uid olmamalı")
	}

	serve := func(name string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		ctx.Params = gin.Params{{Key: "name", Value: name}}
		c.handlePhoto(ctx)
		return w
	}
	ok := serve(strings.TrimPrefix(path, "/answer/api/v1/ib-ldap/foto/"))
	if ok.Code != http.StatusOK || ok.Header().Get("Content-Type") != "image/png" || !bytes.Equal(ok.Body.Bytes(), pngBytes) {
		t.Errorf("resim sunulmadı: %d %s", ok.Code, ok.Header().Get("Content-Type"))
	}
	if w := serve("1234567-0000000000000000"); w.Code != http.StatusNotFound {
		t.Errorf("yanlış hash %d döndü", w.Code)
	}

	// Yanlış parola, resim olmayan yanıt ve kapalı ayar: boş yol, hata yok.
	if got := c.avatarFor(context.Background(), cfg, "1234567", "yanlis", caller); got != "" {
		t.Errorf("yanlış parolada yol = %q", got)
	}
	jsonSrv, _ := fakeYAPI(t, []byte(`{"error":"yok"}`))
	if got := c.avatarFor(context.Background(), Config{PhotoURL: jsonSrv.URL}.normalized(), "1234567", "dogru", caller); got != "" {
		t.Errorf("JSON yanıtta yol = %q", got)
	}
	if got := c.avatarFor(context.Background(), Config{}.normalized(), "1234567", "dogru", caller); got != "" {
		t.Errorf("ayar kapalıyken yol = %q", got)
	}
}

func md5hex(s string) string {
	sum := md5.Sum([]byte(s)) //nolint:gosec // sözleşme
	return hex.EncodeToString(sum[:])
}
