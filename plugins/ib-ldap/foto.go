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
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/apache/answer/plugin"
	"github.com/gin-gonic/gin"
	"github.com/segmentfault/pacman/log"
)

// Profil resmi kurumun resim servisinden (Redis önündeki HTTP ucu) gelir:
// GET <temel adres>/<sicil>. Temel adres eklentinin değil sitenin ayarıdır
// (Yönetim > Kullanıcılar > Ayarlar, eski "Gravatar temel adresi" alanı); çekirdek
// onu giriş isteğinin context'ine PhotoBaseURLKey ile koyar. Kimlik bilgisi
// gönderilmez.
//
// Resim eklentinin KV deposunda durur ve fotoRoute ucundan sunulur; kullanıcı
// kaydına yalnız o ucun yolu yazılır. Her girişte yeniden alınır, yani eski
// sürümden kalan profil resminin yerine geçer.
const (
	// PhotoBaseURLKey çekirdekteki connector_controller.go ile aynı olmalıdır.
	PhotoBaseURLKey = "ib.photo_base_url"

	photoTimeout  = 4 * time.Second
	photoMaxBytes = 2 << 20
	photoGroup    = "foto"
	photoRoute    = "/ib-ldap/foto/"
	photoPath     = "/answer/api/v1" + photoRoute
)

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

// photoURL temel adresin sonuna sicili ekler. Adres boşsa, http(s) değilse ya da
// kurulumdan kalan Gravatar adresiyse (intranetten erişilemez, girişi zaman aşımı
// kadar bekletir) boş döner.
func photoURL(base, sicil string) string {
	base = strings.TrimSpace(base)
	u, err := url.Parse(base)
	if base == "" || err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		strings.Contains(strings.ToLower(u.Host), "gravatar.com") {
		return ""
	}
	return strings.TrimRight(base, "/") + "/" + url.PathEscape(sicil)
}

// avatarFor resmi alır, saklar ve avatar yolunu döndürür. Resim yoksa ya da
// alınamazsa boş döner: giriş bu yüzden asla başarısız olmaz, eski resim de
// yerinde kalır.
func (c *Connector) avatarFor(ctx context.Context, cfg Config, base, sicil string) string {
	kv := c.operator()
	target := photoURL(base, sicil)
	if target == "" || kv == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, photoTimeout)
	defer cancel()
	data, contentType, err := fetchPhoto(ctx, c.httpClient(cfg), target)
	if err != nil {
		log.Warnf("ib-ldap: %s için profil resmi alınamadı: %v", maskSicil(sicil), redact(err, sicil))
		return ""
	}
	if len(data) == 0 {
		log.Infof("ib-ldap: %s için resim servisi resim döndürmedi", maskSicil(sicil))
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

func fetchPhoto(ctx context.Context, client *http.Client, target string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "image/*, application/json;q=0.5")
	res, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return nil, "", nil
	}
	if res.StatusCode/100 != 2 {
		return nil, "", fmt.Errorf("resim servisi HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 2*photoMaxBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("resim okunamadı: %w", err)
	}
	data = decodePhoto(data)
	if len(data) > photoMaxBytes {
		return nil, "", fmt.Errorf("resim %d bayttan büyük", photoMaxBytes)
	}
	// Tür başlığa değil içeriğe göre belirlenir; resim olmayan hiçbir şey
	// saklanmaz ve sunulmaz.
	if len(data) == 0 {
		return nil, "", nil
	}
	return data, http.DetectContentType(data), nil
}

// decodePhoto yanıttan resmi çıkarır. Servis resmi ham bayt olarak, base64 metin
// olarak (Redis'te çoğunlukla böyle durur, "data:image/...;base64," önekiyle ya
// da öneksiz) ya da bir JSON alanının içinde verebilir. Resim bulunamazsa nil.
func decodePhoto(body []byte) []byte {
	if isImage(body) {
		return body
	}
	text := bytes.TrimSpace(body)
	var parsed any
	if json.Unmarshal(text, &parsed) == nil {
		return photoInJSON(parsed, 0)
	}
	return photoFromBase64(string(text))
}

func photoInJSON(v any, depth int) []byte {
	if depth > 4 {
		return nil
	}
	switch t := v.(type) {
	case string:
		return photoFromBase64(t)
	case map[string]any:
		for _, item := range t {
			if data := photoInJSON(item, depth+1); data != nil {
				return data
			}
		}
	case []any:
		for _, item := range t {
			if data := photoInJSON(item, depth+1); data != nil {
				return data
			}
		}
	}
	return nil
}

func photoFromBase64(text string) []byte {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "data:") {
		if _, rest, ok := strings.Cut(text, ","); ok {
			text = rest
		}
	}
	if len(text) < 16 {
		return nil
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if data, err := enc.DecodeString(text); err == nil && isImage(data) {
			return data
		}
	}
	return nil
}

func isImage(data []byte) bool {
	return len(data) > 0 && strings.HasPrefix(http.DetectContentType(data), "image/")
}

// redact hata metninde geçebilecek değeri siler (URL hataları adresi, adres de
// sicili taşır).
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
