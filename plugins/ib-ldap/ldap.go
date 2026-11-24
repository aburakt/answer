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

// Package ibldap kurum LDAP'ı (Active Directory) ile sicil ve parola girişi.
//
// Upstream connector-ldap'tan (apache/answer-plugins) türetildi ve onun yerini
// alır: slug ("ldap_connector"), connector adı ("ldap") ve ayar anahtarları
// aynıdır, kayıtlı ayarlar ve user_external_login satırları geçerli kalır.
// Farkları: şifresiz ldap:// ve sertifika doğrulamasını kapatma seçeneği,
// zaman aşımı, e-postası olmayan kayıt için alan adı, hatanın nedenini taşıyan
// kodlar (LoginErrorCode) ve yönetici için bağlantı denemesi.
package ibldap

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/apache/answer-plugins/ib-ldap/i18n"
	"github.com/apache/answer/plugin"
	"github.com/gin-gonic/gin"
	"github.com/go-ldap/ldap/v3"
	"github.com/segmentfault/pacman/log"
)

const (
	slugName      = "ldap_connector"
	connectorSlug = "ldap"

	securityTLS  = "tls"
	securityNone = "none"

	attrObjectGUID = "objectGUID"

	dialTimeout    = 5 * time.Second
	requestTimeout = 10 * time.Second
)

// Hata kodları: giriş formu kullanıcıya bunlara göre mesaj gösterir, RUNBOOK'ta
// her birinin ne anlama geldiği yazar.
const (
	CodeCredentials = "credentials" // sicil yok ya da parola yanlış
	CodeConfig      = "config"      // eklenti ayarı eksik
	CodeConnect     = "connect"     // sunucuya ulaşılamadı
	CodeTLS         = "tls"         // TLS / sertifika
	CodeBind        = "bind"        // servis hesabı reddedildi
	CodeSearch      = "search"      // arama hatası (Base DN, yetki, çift kayıt)
	CodeMail        = "mail"        // kayıtta e-posta yok
	CodeIdentity    = "identity"    // dış kimlik alanı boş
	CodeOrigin      = "origin"      // istek başka bir adresten geldi
)

// LoginError nedeni kodlanmış giriş hatası. Controller kodu LoginErrorCode ile okur.
type LoginError struct {
	Code string
	Err  error
}

func (e *LoginError) Error() string          { return e.Code + ": " + e.Err.Error() }
func (e *LoginError) Unwrap() error          { return e.Err }
func (e *LoginError) LoginErrorCode() string { return e.Code }

func fail(code, format string, args ...any) *LoginError {
	return &LoginError{Code: code, Err: fmt.Errorf(format, args...)}
}

type Config struct {
	Name           string `json:"name"`
	Server         string `json:"server"`
	Security       string `json:"security"`
	TLSSkipVerify  bool   `json:"tls_skip_verify"`
	TLSCACertPath  string `json:"tls_ca_cert_path"`
	BaseDN         string `json:"base_dn"`
	BindDN         string `json:"bind_dn"`
	BindPassword   string `json:"bind_password"`
	UserAttr       string `json:"user_attr"`
	ExternalIDAttr string `json:"external_id_attr"`
	MailDomain     string `json:"mail_domain"`
	TestUser       string `json:"test_user"`
	// PhotoURL YAPI ucu (ör. https://giris.../yapi). Boşsa profil resmi alınmaz.
	PhotoURL string `json:"photo_url"`
	PhotoApp string `json:"photo_app"`
}

// normalized boşlukları atar ve boş bırakılan alanlara varsayılanı koyar.
func (c Config) normalized() Config {
	for _, p := range []*string{&c.Name, &c.Server, &c.Security, &c.TLSCACertPath, &c.BaseDN,
		&c.BindDN, &c.UserAttr, &c.ExternalIDAttr, &c.MailDomain, &c.TestUser, &c.PhotoURL, &c.PhotoApp} {
		*p = strings.TrimSpace(*p)
	}
	// "sunucu:389" ya da yalnız "sunucu" yazılırsa ldap:// varsayılır.
	if c.Server != "" && !strings.Contains(c.Server, "://") {
		c.Server = "ldap://" + c.Server
	}
	if c.Security != securityNone {
		c.Security = securityTLS
	}
	if c.UserAttr == "" {
		c.UserAttr = "sAMAccountName"
	}
	// Upstream'in varsayılanı entryUUID Active Directory'de yoktur.
	if c.ExternalIDAttr == "" || strings.EqualFold(c.ExternalIDAttr, "entryUUID") && strings.EqualFold(c.UserAttr, "sAMAccountName") {
		c.ExternalIDAttr = c.UserAttr
	}
	c.MailDomain = strings.TrimPrefix(c.MailDomain, "@")
	if c.PhotoApp == "" {
		c.PhotoApp = photoApp
	}
	return c
}

type Connector struct {
	mu     sync.RWMutex
	config Config
	dial   func(Config) (ldap.Client, error)
	kv     photoStore
	// photoClient yalnız testte dolar.
	photoClient *http.Client
}

var (
	_ plugin.Connector = (*Connector)(nil)
	_ plugin.Agent     = (*Connector)(nil)
)

func New() *Connector { return &Connector{dial: dial} }

func init() { plugin.Register(New()) }

func (c *Connector) cfg() Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config.normalized()
}

func (c *Connector) Info() plugin.Info {
	return plugin.Info{
		Name:        plugin.MakeTranslator(i18n.InfoName),
		SlugName:    slugName,
		Description: plugin.MakeTranslator(i18n.InfoDescription),
		Author:      "ib",
		Version:     "0.2.0",
		Link:        "https://github.com/aburakt/answer/tree/main/plugins/ib-ldap",
	}
}

// --- Connector ---

func (c *Connector) ConnectorLogoSVG() string    { return "" }
func (c *Connector) ConnectorSlugName() string   { return connectorSlug }
func (c *Connector) ConnectorRequireState() bool { return true }
func (c *Connector) ConnectorName() plugin.Translator {
	if name := c.cfg().Name; name != "" {
		return plugin.MakeTranslator(name)
	}
	return plugin.MakeTranslator(i18n.ConnectorName)
}

// ConnectorSender: form Answer'ın kendi giriş sayfasındadır (/users/login).
func (c *Connector) ConnectorSender(ctx *plugin.GinContext, _ string) string {
	return "/users/login"
}

func (c *Connector) ConnectorReceiver(ctx *plugin.GinContext, receiverURL string) (plugin.ExternalLoginUserInfo, error) {
	info, err := c.login(ctx.Request, receiverURL,
		photoCaller{ClientIP: ctx.ClientIP(), UserAgent: ctx.Request.UserAgent()})
	if err != nil {
		// Parola ve servis parolası hiçbir zaman log'a yazılmaz.
		log.Errorf("ldap girişi başarısız: %v", err)
	}
	return info, err
}

func (c *Connector) login(r *http.Request, receiverURL string, caller photoCaller) (info plugin.ExternalLoginUserInfo, err error) {
	if err := checkSameOrigin(r, receiverURL); err != nil {
		return info, err
	}
	if err := r.ParseForm(); err != nil {
		return info, fail(CodeCredentials, "form okunamadı: %v", err)
	}
	username, password := strings.TrimSpace(r.PostFormValue("username")), r.PostFormValue("password")
	// Boş parola LDAP'ta "anonim bind" sayılır ve başarılı döner; asla sunucuya gönderilmez.
	if username == "" || password == "" {
		return info, fail(CodeCredentials, "sicil ya da parola boş")
	}

	cfg := c.cfg()
	conn, entry, err := c.find(cfg, username)
	if err != nil {
		return info, err
	}
	defer conn.Close()

	if err := conn.Bind(entry.DN, password); err != nil {
		return info, fail(CodeCredentials, "%s için parola kabul edilmedi: %v", entry.DN, err)
	}
	info, err = userInfo(cfg, entry, username)
	if err != nil {
		return info, err
	}
	info.Avatar = c.avatarFor(r.Context(), cfg, info.Username, password, caller)
	return info, nil
}

// find bağlanır, servis hesabıyla bind eder ve kullanıcıyı arar. Bağlantı açık döner.
func (c *Connector) find(cfg Config, username string) (ldap.Client, *ldap.Entry, error) {
	if cfg.Server == "" || cfg.BaseDN == "" || cfg.BindDN == "" || cfg.BindPassword == "" {
		return nil, nil, fail(CodeConfig, "sunucu, Base DN, Bind DN ve bind parolası girilmeli")
	}
	conn, err := c.dial(cfg)
	if err != nil {
		return nil, nil, err
	}
	if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
		conn.Close()
		return nil, nil, bindError(cfg, err)
	}
	if username == "" {
		return conn, nil, nil
	}
	entry, err := search(conn, cfg, username)
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	return conn, entry, nil
}

// bindError servis hesabı bind hatasını sınıflar. Active Directory şifresiz
// bağlantıda basit bind'i "strong auth required" ile geri çevirir; bu parola
// değil bağlantı güvenliği sorunudur.
func bindError(cfg Config, err error) *LoginError {
	if ldap.IsErrorWithCode(err, ldap.LDAPResultStrongAuthRequired) ||
		ldap.IsErrorWithCode(err, ldap.LDAPResultConfidentialityRequired) {
		return fail(CodeTLS, "sunucu şifresiz bağlantıda girişi kabul etmiyor, şifreli bağlantı seçilmeli: %v", err)
	}
	return fail(CodeBind, "servis hesabı %q reddedildi: %v", cfg.BindDN, err)
}

func search(conn ldap.Client, cfg Config, username string) (*ldap.Entry, error) {
	req := ldap.NewSearchRequest(cfg.BaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		2, int(requestTimeout.Seconds()), false,
		fmt.Sprintf("(%s=%s)", cfg.UserAttr, ldap.EscapeFilter(username)),
		[]string{"uid", "cn", "mail", "displayName", "sAMAccountName", cfg.UserAttr, cfg.ExternalIDAttr}, nil)
	res, err := conn.Search(req)
	if err != nil && !ldap.IsErrorWithCode(err, ldap.LDAPResultSizeLimitExceeded) {
		return nil, fail(CodeSearch, "%q altında arama başarısız: %v", cfg.BaseDN, err)
	}
	switch n := len(res.Entries); {
	case n == 0:
		return nil, fail(CodeCredentials, "%s=%s kaydı %q altında yok", cfg.UserAttr, username, cfg.BaseDN)
	case n > 1 || err != nil:
		return nil, fail(CodeSearch, "%s=%s için birden fazla kayıt var", cfg.UserAttr, username)
	}
	return res.Entries[0], nil
}

func userInfo(cfg Config, entry *ldap.Entry, typed string) (info plugin.ExternalLoginUserInfo, err error) {
	first := func(attrs ...string) string {
		for _, a := range attrs {
			if v := strings.TrimSpace(entry.GetEqualFoldAttributeValue(a)); v != "" {
				return v
			}
		}
		return ""
	}
	info.Username = first("sAMAccountName", "uid", cfg.UserAttr)
	if info.Username == "" {
		info.Username = typed
	}
	info.DisplayName = first("displayName", "cn")
	if info.DisplayName == "" {
		info.DisplayName = info.Username
	}

	if strings.EqualFold(cfg.ExternalIDAttr, attrObjectGUID) {
		raw := entry.GetEqualFoldRawAttributeValue(cfg.ExternalIDAttr)
		if len(raw) != 16 {
			return info, fail(CodeIdentity, "%s: %s alanı yok ya da geçersiz", entry.DN, cfg.ExternalIDAttr)
		}
		info.ExternalID = formatObjectGUID(raw)
	} else if info.ExternalID = first(cfg.ExternalIDAttr); info.ExternalID == "" {
		return info, fail(CodeIdentity, "%s: %s alanı boş", entry.DN, cfg.ExternalIDAttr)
	}

	info.Email = strings.ToLower(first("mail"))
	if info.Email == "" && cfg.MailDomain != "" {
		info.Email = strings.ToLower(info.Username) + "@" + cfg.MailDomain
	}
	if info.Email == "" {
		return info, fail(CodeMail, "%s: mail alanı boş ve e-posta alan adı ayarlanmamış", entry.DN)
	}
	return info, nil
}

func formatObjectGUID(guid []byte) string {
	return fmt.Sprintf("%08x-%04x-%04x-%x-%x",
		binary.LittleEndian.Uint32(guid[0:4]), binary.LittleEndian.Uint16(guid[4:6]),
		binary.LittleEndian.Uint16(guid[6:8]), guid[8:10], guid[10:16])
}

// checkSameOrigin CSRF'e karşı isteğin sitenin kendisinden geldiğini doğrular.
// Origin (yoksa Referer) ya site adresiyle ya da isteğin Host başlığıyla aynı
// host'ta olmalı. Şema karşılaştırılmaz: TLS'i sonlandıran proxy arkasında
// Answer isteği http olarak görür.
func checkSameOrigin(r *http.Request, receiverURL string) error {
	source := r.Header.Get("Origin")
	if source == "" || source == "null" {
		source = r.Header.Get("Referer")
	}
	if source == "" {
		return fail(CodeOrigin, "Origin ve Referer başlığı yok")
	}
	su, err := url.Parse(source)
	if err != nil || su.Host == "" {
		return fail(CodeOrigin, "Origin/Referer okunamadı: %q", source)
	}
	allowed := []string{r.Host}
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		allowed = append(allowed, h)
	}
	if ru, err := url.Parse(receiverURL); err == nil {
		allowed = append(allowed, ru.Host)
	}
	for _, host := range allowed {
		if host != "" && strings.EqualFold(su.Host, host) {
			return nil
		}
	}
	return fail(CodeOrigin, "istek %q adresinden geldi; beklenen %s", su.Host, strings.Join(allowed, " ya da "))
}

// caPool ayardaki CA dosyasını sistem deposuna ekler; dosya yoksa nil döner
// (sistem deposu kullanılır).
func caPool(cfg Config) (*x509.CertPool, error) {
	if cfg.TLSCACertPath == "" || cfg.TLSSkipVerify {
		return nil, nil
	}
	pem, err := os.ReadFile(cfg.TLSCACertPath)
	if err != nil {
		return nil, fail(CodeTLS, "CA sertifikası okunamadı: %v", err)
	}
	pool, _ := x509.SystemCertPool()
	if pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fail(CodeTLS, "%s içinde PEM sertifika yok", cfg.TLSCACertPath)
	}
	return pool, nil
}

func dial(cfg Config) (ldap.Client, error) {
	u, err := url.Parse(cfg.Server)
	if err != nil || u.Hostname() == "" {
		return nil, fail(CodeConfig, "sunucu adresi okunamadı: %q", cfg.Server)
	}
	if u.Scheme != "ldap" && u.Scheme != "ldaps" {
		return nil, fail(CodeConfig, "sunucu adresi ldap:// ya da ldaps:// ile başlamalı: %q", cfg.Server)
	}
	tlsConfig := &tls.Config{ServerName: u.Hostname(), InsecureSkipVerify: cfg.TLSSkipVerify} //nolint:gosec // yönetici ayarı
	pool, err := caPool(cfg)
	if err != nil {
		return nil, err
	}
	tlsConfig.RootCAs = pool

	conn, err := ldap.DialURL(cfg.Server, ldap.DialWithDialer(&net.Dialer{Timeout: dialTimeout}),
		ldap.DialWithTLSConfig(tlsConfig))
	if err != nil {
		return nil, fail(classify(err), "%s bağlantısı kurulamadı: %v", cfg.Server, err)
	}
	conn.SetTimeout(requestTimeout)
	if u.Scheme == "ldap" && cfg.Security != securityNone {
		if err := conn.StartTLS(tlsConfig); err != nil {
			conn.Close()
			return nil, fail(CodeTLS, "%s StartTLS başarısız: %v", cfg.Server, err)
		}
	}
	return conn, nil
}

// classify bağlantı hatasının TLS kaynaklı olup olmadığını ayırır.
func classify(err error) string {
	var certErr *tls.CertificateVerificationError
	var recordErr tls.RecordHeaderError
	if errors.As(err, &certErr) || errors.As(err, &recordErr) {
		return CodeTLS
	}
	if msg := err.Error(); strings.Contains(msg, "x509:") || strings.Contains(msg, "tls:") {
		return CodeTLS
	}
	return CodeConnect
}

// --- Config ---

func (c *Connector) ConfigFields() []plugin.ConfigField {
	c.mu.RLock()
	raw := c.config
	c.mu.RUnlock()
	cfg := raw.normalized()
	input := func(name, value string, required bool, kind plugin.InputType) plugin.ConfigField {
		return plugin.ConfigField{
			Name: name, Type: plugin.ConfigTypeInput, Required: required, Value: value,
			Title:       plugin.MakeTranslator(i18n.ConfigTitle(name)),
			Description: plugin.MakeTranslator(i18n.ConfigDescription(name)),
			UIOptions:   plugin.ConfigFieldUIOptions{InputType: kind},
		}
	}
	text, secret := plugin.InputTypeText, plugin.InputTypePassword
	return []plugin.ConfigField{
		input("name", raw.Name, false, text),
		input("server", raw.Server, true, text),
		{
			Name: "security", Type: plugin.ConfigTypeSelect, Value: cfg.Security,
			Title:       plugin.MakeTranslator(i18n.ConfigTitle("security")),
			Description: plugin.MakeTranslator(i18n.ConfigDescription("security")),
			Options: []plugin.ConfigFieldOption{
				{Value: securityTLS, Label: plugin.MakeTranslator(i18n.SecurityTLS)},
				{Value: securityNone, Label: plugin.MakeTranslator(i18n.SecurityNone)},
			},
		},
		{
			Name: "tls_skip_verify", Type: plugin.ConfigTypeSwitch, Value: raw.TLSSkipVerify,
			Title:       plugin.MakeTranslator(i18n.ConfigTitle("tls_skip_verify")),
			Description: plugin.MakeTranslator(i18n.ConfigDescription("tls_skip_verify")),
			UIOptions:   plugin.ConfigFieldUIOptions{Label: plugin.MakeTranslator(i18n.ConfigLabel("tls_skip_verify"))},
		},
		input("tls_ca_cert_path", raw.TLSCACertPath, false, text),
		input("base_dn", raw.BaseDN, true, text),
		input("bind_dn", raw.BindDN, true, text),
		input("bind_password", raw.BindPassword, true, secret),
		input("user_attr", cfg.UserAttr, true, text),
		input("external_id_attr", cfg.ExternalIDAttr, true, text),
		input("mail_domain", raw.MailDomain, false, text),
		input("test_user", raw.TestUser, false, text),
		input("photo_url", raw.PhotoURL, false, text),
		input("photo_app", cfg.PhotoApp, false, text),
		{
			Name: "test", Type: plugin.ConfigTypeButton,
			Title:       plugin.MakeTranslator(i18n.ConfigTitle("test")),
			Description: plugin.MakeTranslator(i18n.ConfigDescription("test")),
			UIOptions: plugin.ConfigFieldUIOptions{
				Text:    plugin.MakeTranslator(i18n.ConfigLabel("test")),
				Variant: "outline-secondary",
				Action: &plugin.UIOptionAction{
					Url: "/answer/admin/api/ib-ldap/test", Method: "get",
					OnComplete: &plugin.OnCompleteAction{ToastReturnMessage: true},
				},
			},
		},
	}
}

func (c *Connector) ConfigReceiver(config []byte) error {
	var cfg Config
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config json: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config = cfg
	return nil
}

// --- Agent: yönetici için bağlantı denemesi ---

func (c *Connector) RegisterUnAuthRouter(r *gin.RouterGroup) {
	r.GET(photoRoute+":name", c.handlePhoto)
}
func (c *Connector) RegisterAuthUserRouter(*gin.RouterGroup) {}
func (c *Connector) RegisterAuthAdminRouter(r *gin.RouterGroup) {
	r.GET("/ib-ldap/test", c.handleTest)
}

// handleTest kayıtlı ayarla bağlanır, servis hesabını dener ve "Deneme sicili"
// doluysa o kaydı arar. Kullanıcı parolası gerekmez. Yanıt Answer'ın zarfıyla
// döner: başarıda bildirim, hatada pencere açılır.
func (c *Connector) handleTest(ctx *gin.Context) {
	tr := func(key string) string { return plugin.Translate(ctx, key) }
	cfg := c.cfg()
	msg, err := c.test(cfg)
	if err != nil {
		code := CodeConnect
		var le *LoginError
		if errors.As(err, &le) {
			code = le.Code
		}
		log.Errorf("ldap denemesi başarısız: %v", err)
		ctx.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "reason": "ib_ldap." + code,
			"msg": fmt.Sprintf("%s %s (%v)", tr(i18n.TestFailed), tr(i18n.ReasonPrefix+code), err)})
		return
	}
	text := tr(i18n.TestOK)
	if msg != "" {
		text += " " + fmt.Sprintf(tr(i18n.TestUserOK), msg)
	}
	ctx.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "reason": "base.success", "msg": text,
		"data": gin.H{"message": text}})
}

func (c *Connector) test(cfg Config) (string, error) {
	conn, entry, err := c.find(cfg, cfg.TestUser)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if entry == nil {
		return "", nil
	}
	info, err := userInfo(cfg, entry, cfg.TestUser)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s, %s, %s=%s", info.DisplayName, info.Email, cfg.ExternalIDAttr, info.ExternalID), nil
}
