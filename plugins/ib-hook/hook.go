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

// Package ibhook imzalı webhook'larla Answer'da konu açan ve güncelleyen eklenti.
// Sözleşme: giris /answer-config sayfası. İmza Standard Webhooks
// (https://www.standardwebhooks.com/): webhook-id, webhook-timestamp,
// webhook-signature = "v1,<base64(HMAC-SHA256(secret, id.timestamp.gövde))>".
package ibhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/apache/answer-plugins/ib-hook/i18n"
	"github.com/apache/answer/plugin"
	"github.com/gin-gonic/gin"
)

const (
	slugName      = "ib_hook"
	maxBody       = 256 << 10
	timestampSkew = 5 * time.Minute
)

// Source hook gönderebilen bir uygulama (admin > eklenti ayarları).
type Source struct {
	ID       string   `json:"id"`
	BotEmail string   `json:"bot_email"`
	Tags     []string `json:"tags"`
	LinkText string   `json:"link_text"`
}

// Event gelen olay gövdesi.
type Event struct {
	Type    string   `json:"type"` // topic.upsert | topic.close
	Ref     string   `json:"ref"`
	Version int      `json:"version"`
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
	URL     string   `json:"url"`
	Message string   `json:"message"`
}

// topic ref -> soru eşlemesi (KV storage'da JSON).
type topic struct {
	QuestionID string `json:"question_id"`
	Version    int    `json:"version"`
	Closed     bool   `json:"closed"`
}

// kvStore *plugin.KVOperator'ın kullandığımız kısmı (testte bellek ikizi).
type kvStore interface {
	Get(ctx context.Context, p plugin.KVParams) (string, error)
	Set(ctx context.Context, p plugin.KVParams) error
}

// Hook eklentinin kendisi: Agent (rotalar), Config, KVStorage, ContentWriter.
type Hook struct {
	mu      sync.RWMutex
	sources map[string]Source
	raw     string
	kv      kvStore
	content plugin.ContentFunc
	getenv  func(string) string
	now     func() time.Time

	locks sync.Map // source+ref -> *sync.Mutex
	seen  seenIDs
}

func New() *Hook {
	return &Hook{sources: map[string]Source{}, getenv: os.Getenv, now: time.Now}
}

func init() {
	h := New()
	plugin.Register(h)
	plugin.RegisterContentWriter(h)
}

func (h *Hook) Info() plugin.Info {
	return plugin.Info{
		Name:        plugin.MakeTranslator(i18n.InfoName),
		SlugName:    slugName,
		Description: plugin.MakeTranslator(i18n.InfoDescription),
		Author:      "ib",
		Version:     "0.1.0",
		Link:        "https://github.com/aburakt/answer/tree/main/plugins/ib-hook",
	}
}

// --- Config ---

func (h *Hook) ConfigFields() []plugin.ConfigField {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return []plugin.ConfigField{{
		Name:        "sources",
		Type:        plugin.ConfigTypeTextarea,
		Title:       plugin.MakeTranslator(i18n.ConfigSourcesTitle),
		Description: plugin.MakeTranslator(i18n.ConfigSourcesDescription),
		Value:       h.raw,
		UIOptions:   plugin.ConfigFieldUIOptions{Rows: "8"},
	}}
}

func (h *Hook) ConfigReceiver(config []byte) error {
	var c struct {
		Sources string `json:"sources"`
	}
	if err := json.Unmarshal(config, &c); err != nil {
		return err
	}
	parsed, err := parseSources(c.Sources)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.raw, h.sources = c.Sources, parsed
	return nil
}

func parseSources(raw string) (map[string]Source, error) {
	out := map[string]Source{}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	var list []Source
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, fmt.Errorf("kaynaklar JSON listesi olmalı: %w", err)
	}
	for _, s := range list {
		if s.ID == "" || s.BotEmail == "" || strings.ContainsAny(s.ID, "/ ") {
			return nil, fmt.Errorf("kaynak %q: id ve bot_email zorunlu", s.ID)
		}
		out[s.ID] = s
	}
	return out, nil
}

// --- KVStorage / ContentWriter ---

func (h *Hook) SetOperator(op *plugin.KVOperator) { h.kv = op }

func (h *Hook) RegisterContentFunc(_ context.Context, fn plugin.ContentFunc) { h.content = fn }

// --- Agent ---

func (h *Hook) RegisterUnAuthRouter(r *gin.RouterGroup) {
	r.POST("/hooks/:source", h.receive)
	r.GET("/hooks/:source/topics/*ref", h.redirect)
}

func (h *Hook) RegisterAuthUserRouter(*gin.RouterGroup)  {}
func (h *Hook) RegisterAuthAdminRouter(*gin.RouterGroup) {}

func (h *Hook) source(id string) (Source, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	s, ok := h.sources[id]
	return s, ok
}

// secret ANSWER_HOOK_SECRET_<ID> (büyük harf, '-' yerine '_').
func (h *Hook) secret(id string) []byte {
	v := h.getenv("ANSWER_HOOK_SECRET_" + strings.ToUpper(strings.ReplaceAll(id, "-", "_")))
	if after, ok := strings.CutPrefix(v, "whsec_"); ok {
		if b, err := base64.StdEncoding.DecodeString(after); err == nil {
			return b
		}
	}
	return []byte(v)
}

func fail(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"ok": false, "error": msg})
}

func (h *Hook) receive(c *gin.Context) {
	// Agent rotaları eklenti kapalıyken de bağlı kalır; kontrol burada.
	if !plugin.StatusManager.IsEnabled(slugName) {
		// 503: gönderen (Kılavuz) 5xx'i yeniden dener, 404'te olayı bırakır.
		fail(c, http.StatusServiceUnavailable, "hook eklentisi kapalı")
		return
	}
	src, ok := h.source(c.Param("source"))
	secret := h.secret(c.Param("source"))
	if !ok {
		fail(c, http.StatusNotFound, "bilinmeyen kaynak")
		return
	}
	if len(secret) == 0 {
		// Kaynak tanımlı ama secret henüz verilmemiş: kurulum eksiği, olay kaybolmasın.
		fail(c, http.StatusServiceUnavailable, "kaynağın secret'ı tanımlı değil")
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		fail(c, http.StatusRequestEntityTooLarge, "gövde çok büyük")
		return
	}
	id := c.GetHeader("webhook-id")
	if err := verify(secret, id, c.GetHeader("webhook-timestamp"), c.GetHeader("webhook-signature"), body, h.now()); err != nil {
		fail(c, http.StatusUnauthorized, err.Error())
		return
	}
	if !h.seen.add(src.ID+":"+id, h.now()) {
		c.JSON(http.StatusOK, gin.H{"ok": true, "duplicate": true})
		return
	}
	var ev Event
	if err := json.Unmarshal(body, &ev); err != nil || ev.Ref == "" || len(ev.Ref) > 120 {
		h.seen.remove(src.ID + ":" + id)
		fail(c, http.StatusBadRequest, "geçersiz olay")
		return
	}
	res, status, err := h.apply(c, src, ev)
	if err != nil {
		h.seen.remove(src.ID + ":" + id) // tekrar denenebilsin
		fail(c, status, err.Error())
		return
	}
	c.JSON(http.StatusOK, res)
}

// verify Standard Webhooks imzasını ve zaman damgasını doğrular.
func verify(secret []byte, id, ts, sigHeader string, body []byte, now time.Time) error {
	if id == "" || ts == "" || sigHeader == "" {
		return errors.New("imza başlıkları eksik")
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errors.New("geçersiz zaman damgası")
	}
	if d := now.Sub(time.Unix(sec, 0)); d > timestampSkew || d < -timestampSkew {
		return errors.New("zaman damgası tolerans dışında")
	}
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(id + "." + ts + "."))
	m.Write(body)
	want := m.Sum(nil)
	for _, part := range strings.Fields(sigHeader) {
		v, sig, ok := strings.Cut(part, ",")
		if !ok || v != "v1" {
			continue
		}
		got, err := base64.StdEncoding.DecodeString(sig)
		if err == nil && hmac.Equal(got, want) {
			return nil
		}
	}
	return errors.New("imza geçersiz")
}

func (h *Hook) lock(key string) func() {
	v, _ := h.locks.LoadOrStore(key, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func (h *Hook) load(ctx context.Context, src, ref string) (topic, bool, error) {
	v, err := h.kv.Get(ctx, plugin.KVParams{Group: "topic:" + src, Key: ref})
	if errors.Is(err, plugin.ErrKVKeyNotFound) {
		return topic{}, false, nil
	}
	if err != nil {
		return topic{}, false, err
	}
	var t topic
	if err := json.Unmarshal([]byte(v), &t); err != nil {
		return topic{}, false, err
	}
	return t, true, nil
}

func (h *Hook) save(ctx context.Context, src, ref string, t topic) error {
	b, _ := json.Marshal(t)
	return h.kv.Set(ctx, plugin.KVParams{Group: "topic:" + src, Key: ref, Value: string(b)})
}

func (h *Hook) apply(c *gin.Context, src Source, ev Event) (gin.H, int, error) {
	if h.kv == nil || h.content == nil {
		return nil, http.StatusServiceUnavailable, errors.New("eklenti henüz hazır değil")
	}
	defer h.lock(src.ID + "\x00" + ev.Ref)()
	t, exists, err := h.load(c, src.ID, ev.Ref)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	switch ev.Type {
	case "topic.upsert":
		if ev.Title == "" || ev.Content == "" {
			return nil, http.StatusBadRequest, errors.New("title ve content zorunlu")
		}
		if exists && ev.Version <= t.Version {
			return gin.H{"ok": true, "question_id": t.QuestionID, "ignored": "eski ya da aynı sürüm"}, 0, nil
		}
		info := plugin.QuestionWriteInfo{
			Title: ev.Title, Content: body(src, ev), Tags: mergeTags(src.Tags, ev.Tags), UserEmail: src.BotEmail,
			EditSummary: fmt.Sprintf("Kaynak güncellendi: sürüm %d", ev.Version),
		}
		if !exists {
			id, err := h.content.CreateQuestion(c, info)
			if err != nil {
				return nil, http.StatusUnprocessableEntity, err
			}
			t = topic{QuestionID: id, Version: ev.Version}
		} else {
			if t.Closed {
				if err := h.content.ReopenQuestion(c, t.QuestionID, src.BotEmail); err != nil {
					return nil, http.StatusUnprocessableEntity, err
				}
				t.Closed = false
			}
			if err := h.content.UpdateQuestion(c, t.QuestionID, info); err != nil {
				return nil, http.StatusUnprocessableEntity, err
			}
			t.Version = ev.Version
		}
	case "topic.close":
		if !exists {
			return nil, http.StatusNotFound, errors.New("bu ref için konu yok")
		}
		if !t.Closed {
			if err := h.content.CloseQuestion(c, t.QuestionID, src.BotEmail, ev.Message); err != nil {
				return nil, http.StatusUnprocessableEntity, err
			}
			t.Closed = true
		}
	default:
		return nil, http.StatusBadRequest, fmt.Errorf("bilinmeyen olay türü %q", ev.Type)
	}
	if err := h.save(c, src.ID, ev.Ref, t); err != nil {
		return nil, http.StatusInternalServerError, err
	}
	return gin.H{"ok": true, "question_id": t.QuestionID, "url": questionURL(t.QuestionID)}, 0, nil
}

func body(src Source, ev Event) string {
	if ev.URL == "" {
		return ev.Content
	}
	text := src.LinkText
	if text == "" {
		text = "Kaynağa git"
	}
	return ev.Content + "\n\n---\n\n[" + text + "](" + ev.URL + ")"
}

func mergeTags(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range append(append([]string{}, a...), b...) {
		t = strings.TrimSpace(t)
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

func questionURL(id string) string { return plugin.SiteURL() + "/questions/" + id }

// redirect kaynak uygulamanın ref ile konuya link verebilmesi için.
func (h *Hook) redirect(c *gin.Context) {
	if !plugin.StatusManager.IsEnabled(slugName) || h.kv == nil {
		c.Status(http.StatusNotFound)
		return
	}
	ref := strings.TrimPrefix(c.Param("ref"), "/")
	t, ok, err := h.load(c, c.Param("source"), ref)
	if err != nil || !ok {
		c.Status(http.StatusNotFound)
		return
	}
	c.Redirect(http.StatusFound, questionURL(t.QuestionID))
}

// seenIDs webhook-id tekrar koruması (bellekte, tolerans süresi kadar).
type seenIDs struct {
	mu  sync.Mutex
	ids map[string]time.Time
}

func (s *seenIDs) add(id string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ids == nil {
		s.ids = map[string]time.Time{}
	}
	for k, t := range s.ids {
		if now.Sub(t) > 2*timestampSkew {
			delete(s.ids, k)
		}
	}
	if _, dup := s.ids[id]; dup {
		return false
	}
	s.ids[id] = now
	return true
}

func (s *seenIDs) remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.ids, id)
}
