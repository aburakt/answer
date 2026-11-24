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
	"crypto/md5" //nolint:gosec // YAPI sözleşmesi: X-User-Agent = md5(userAgent)
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/apache/answer/plugin"
	"github.com/gin-gonic/gin"
	"github.com/segmentfault/pacman/log"
)

// Profil resmi giris'teki istekle aynı yoldan gelir (giris
// src/services/directoryService.ts fetchPhoto): YAPI'ye
// PERSONEL_WEB · personel · get_foto_from_redis_by_sicil görevi, X-Token,
// X-Uid, X-Clientip ve X-User-Agent = md5(userAgent) başlıklarıyla POST edilir.
//
// Forumda YAPI oturumu yoktur, o yüzden token giriş anında, kullanıcının az önce
// LDAP'ın kabul ettiği sicil ve parolasıyla YAPI "giris" görevinden alınır
// (giris src/services/authService.ts login). Resim eklentinin KV deposunda durur
// ve fotoRoute ucundan sunulur; kullanıcı kaydına yalnız o ucun yolu yazılır.
const (
	photoTimeout  = 4 * time.Second
	photoMaxBytes = 2 << 20
	photoGroup    = "foto"
	photoRoute    = "/ib-ldap/foto/"
	photoPath     = "/answer/api/v1" + photoRoute
	photoApp      = "GIRIS_DASHBOARD"
)

// photoCaller isteği yapan tarayıcının YAPI'ye aktarılan bilgileri.
type photoCaller struct {
	ClientIP  string
	UserAgent string
}

type storedPhoto struct {
	Type string `json:"type"`
	Hash string `json:"hash"`
	Data string `json:"data"`
}

// photoStore eklentinin KV deposunun kullanılan kısmı (testte bellek içi).
type photoStore interface {
	Get(ctx context.Context, params plugin.KVParams) (string, error)
	Set(ctx context.Context, params plugin.KVParams) error
}

var _ plugin.KVStorage = (*Connector)(nil)

func (c *Connector) SetOperator(operator *plugin.KVOperator) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.kv = operator
}

func (c *Connector) operator() photoStore {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.kv
}

// avatarFor resmi YAPI'den alır, saklar ve avatar yolunu döndürür. Resim yoksa
// ya da alınamazsa boş döner: giriş bu yüzden asla başarısız olmaz, eski resim
// de yerinde kalır. Parola ve token log'a yazılmaz.
func (c *Connector) avatarFor(ctx context.Context, cfg Config, sicil, password string, caller photoCaller) string {
	kv := c.operator()
	if cfg.PhotoURL == "" || kv == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, photoTimeout)
	defer cancel()
	data, contentType, err := fetchPhoto(ctx, c.httpClient(cfg), cfg, sicil, password, caller)
	if err != nil {
		log.Warnf("ib-ldap: %s için profil resmi alınamadı: %v", maskSicil(sicil), err)
		return ""
	}
	if len(data) == 0 {
		return ""
	}
	sum := sha256.Sum256(data)
	photo := storedPhoto{Type: contentType, Hash: hex.EncodeToString(sum[:8]), Data: base64.StdEncoding.EncodeToString(data)}
	value, _ := json.Marshal(photo)
	if err := kv.Set(ctx, plugin.KVParams{Group: photoGroup, Key: sicil, Value: string(value)}); err != nil {
		log.Warnf("ib-ldap: %s için profil resmi saklanamadı: %v", maskSicil(sicil), err)
		return ""
	}
	return photoPath + sicil + "-" + photo.Hash
}

func (c *Connector) httpClient(cfg Config) *http.Client {
	if c.photoClient != nil {
		return c.photoClient
	}
	tlsConfig := &tls.Config{InsecureSkipVerify: cfg.TLSSkipVerify} //nolint:gosec // yönetici ayarı
	if pool, err := caPool(cfg); err == nil && pool != nil {
		tlsConfig.RootCAs = pool
	}
	return &http.Client{Timeout: photoTimeout, Transport: &http.Transport{TLSClientConfig: tlsConfig, Proxy: nil}}
}

func fetchPhoto(ctx context.Context, client *http.Client, cfg Config, sicil, password string, caller photoCaller) ([]byte, string, error) {
	uaHash := md5.Sum([]byte(caller.UserAgent)) //nolint:gosec // sözleşme
	post := func(token string, withUID bool, body map[string]string) (*http.Response, error) {
		payload, _ := json.Marshal(body)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.PhotoURL, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Token", token)
		req.Header.Set("X-Clientip", caller.ClientIP)
		req.Header.Set("X-User-Agent", hex.EncodeToString(uaHash[:]))
		if withUID {
			req.Header.Set("X-Uid", sicil)
		}
		return client.Do(req)
	}

	// 1. Oturum: giris'in login isteği (X-Token: "login", X-Uid yok).
	res, err := post("login", false, map[string]string{
		"application": cfg.PhotoApp, "uid": sicil, "maintask": "giris", "password": password,
	})
	if err != nil {
		return nil, "", fmt.Errorf("YAPI oturumu: %w", redact(err, password))
	}
	var login map[string]any
	err = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&login)
	res.Body.Close()
	if res.StatusCode/100 != 2 || err != nil {
		return nil, "", fmt.Errorf("YAPI oturumu: HTTP %d", res.StatusCode)
	}
	token, _ := login["X-Token"].(string)
	if token == "" {
		token, _ = login["token"].(string)
	}
	if ok, _ := login["login"].(bool); !ok || token == "" || token == "404" {
		return nil, "", fmt.Errorf("YAPI oturumu kabul edilmedi")
	}

	// 2. Resim: giris'in fetchPhoto isteği.
	res, err = post(token, true, map[string]string{
		"application": "PERSONEL_WEB", "maintask": "personel",
		"subtask": "get_foto_from_redis_by_sicil", "uid": sicil, "sicil": sicil,
	})
	if err != nil {
		return nil, "", fmt.Errorf("YAPI resim: %w", redact(err, token))
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return nil, "", fmt.Errorf("YAPI resim: HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, photoMaxBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("YAPI resim okunamadı: %w", err)
	}
	if len(data) > photoMaxBytes {
		return nil, "", fmt.Errorf("YAPI resim %d bayttan büyük", photoMaxBytes)
	}
	// Boş gövde ya da JSON hata zarfı: resim yok. Tür başlığa değil içeriğe göre
	// belirlenir; resim olmayan hiçbir şey saklanmaz ve sunulmaz.
	contentType := http.DetectContentType(data)
	if len(data) == 0 || !strings.HasPrefix(contentType, "image/") {
		return nil, "", nil
	}
	return data, contentType, nil
}

// redact hata metninde geçebilecek gizli değeri siler (URL hataları gövdeyi
// taşımaz; bu yalnızca ek güvencedir).
func redact(err error, secret string) error {
	if secret == "" {
		return err
	}
	return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), secret, "***"))
}

// maskSicil log için: son iki hane dışında yıldız.
func maskSicil(sicil string) string {
	if len(sicil) <= 2 {
		return "**"
	}
	return strings.Repeat("*", len(sicil)-2) + sicil[len(sicil)-2:]
}

// handlePhoto saklanan resmi sunar. Yol "<sicil>-<hash>" biçimindedir: hash
// tutmuyorsa 404 döner, yani sicil bilmek resmi görmeye yetmez ve resim değişince
// URL de değişir (tarayıcı önbelleği süresiz tutulabilir).
func (c *Connector) handlePhoto(ctx *gin.Context) {
	sicil, hash, ok := strings.Cut(ctx.Param("name"), "-")
	kv := c.operator()
	if !ok || kv == nil {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}
	value, err := kv.Get(ctx, plugin.KVParams{Group: photoGroup, Key: sicil})
	var photo storedPhoto
	if err != nil || json.Unmarshal([]byte(value), &photo) != nil || photo.Hash != hash ||
		!strings.HasPrefix(photo.Type, "image/") {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}
	data, err := base64.StdEncoding.DecodeString(photo.Data)
	if err != nil {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}
	ctx.Header("Cache-Control", "private, max-age=31536000, immutable")
	ctx.Header("X-Content-Type-Options", "nosniff")
	ctx.Data(http.StatusOK, photo.Type, data)
}
