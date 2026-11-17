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

package ibhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/apache/answer/plugin"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
	"os"
)

type memKV map[string]string

func (m memKV) Get(_ context.Context, p plugin.KVParams) (string, error) {
	v, ok := m[p.Group+"|"+p.Key]
	if !ok {
		return "", plugin.ErrKVKeyNotFound
	}
	return v, nil
}

func (m memKV) Set(_ context.Context, p plugin.KVParams) error {
	m[p.Group+"|"+p.Key] = p.Value
	return nil
}

type fakeContent struct {
	created, updated, closed, reopened int
	fail                               bool
}

func (f *fakeContent) CreateQuestion(context.Context, plugin.QuestionWriteInfo) (string, error) {
	if f.fail {
		return "", errors.New("yazılamadı")
	}
	f.created++
	return "Q1", nil
}
func (f *fakeContent) UpdateQuestion(_ context.Context, _ string, info plugin.QuestionWriteInfo) error {
	f.updated++
	if !strings.Contains(info.EditSummary, "sürüm") {
		return errors.New("özet yok")
	}
	return nil
}
func (f *fakeContent) CloseQuestion(context.Context, string, string, string) error {
	f.closed++
	return nil
}
func (f *fakeContent) ReopenQuestion(context.Context, string, string) error { f.reopened++; return nil }

var now = time.Unix(1_800_000_000, 0)

func setup(t *testing.T) (*Hook, *fakeContent, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := New()
	h.now = func() time.Time { return now }
	h.getenv = func(k string) string {
		if k == "ANSWER_HOOK_SECRET_KILAVUZ" {
			return "whsec_" + base64.StdEncoding.EncodeToString([]byte("gizli"))
		}
		return ""
	}
	if err := h.ConfigReceiver([]byte(`{"sources":"[{\"id\":\"kilavuz\",\"bot_email\":\"bot@x\",\"tags\":[\"kilavuz\"]}]"}`)); err != nil {
		t.Fatal(err)
	}
	fc := &fakeContent{}
	h.kv, h.content = memKV{}, fc
	plugin.StatusManager.Enable(slugName, true)
	r := gin.New()
	h.RegisterUnAuthRouter(r.Group("/answer/api/v1"))
	return h, fc, r
}

func send(r *gin.Engine, src, id, body string, secret []byte, ts time.Time) *httptest.ResponseRecorder {
	tss := strconv.FormatInt(ts.Unix(), 10)
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(id + "." + tss + "." + body))
	req := httptest.NewRequest("POST", "/answer/api/v1/hooks/"+src, strings.NewReader(body))
	req.Header.Set("webhook-id", id)
	req.Header.Set("webhook-timestamp", tss)
	req.Header.Set("webhook-signature", "v1,"+base64.StdEncoding.EncodeToString(m.Sum(nil)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestUpsertLifecycle(t *testing.T) {
	_, fc, r := setup(t)
	key := []byte("gizli")
	up := func(v int) string {
		return `{"type":"topic.upsert","ref":"training:abc","version":` + strconv.Itoa(v) +
			`,"title":"İlk yardım eğitimi","content":"İçerik","url":"https://kilavuz/t/abc"}`
	}
	if w := send(r, "kilavuz", "m1", up(1), key, now); w.Code != 200 || fc.created != 1 {
		t.Fatalf("ilk: %d %s", w.Code, w.Body)
	}
	// Aynı mesaj tekrar: yeni konu açılmaz.
	if w := send(r, "kilavuz", "m1", up(1), key, now); w.Code != 200 || fc.created != 1 {
		t.Fatalf("tekrar: %d", w.Code)
	}
	// Aynı sürüm farklı id ile: yok sayılır.
	send(r, "kilavuz", "m2", up(1), key, now)
	if fc.updated != 0 {
		t.Fatal("aynı sürüm güncellememeli")
	}
	send(r, "kilavuz", "m3", up(2), key, now)
	if fc.updated != 1 {
		t.Fatal("yeni sürüm güncellemeli")
	}
	send(r, "kilavuz", "m4", `{"type":"topic.close","ref":"training:abc","message":"Yayından kalktı"}`, key, now)
	send(r, "kilavuz", "m5", up(3), key, now)
	if fc.closed != 1 || fc.reopened != 1 || fc.updated != 2 {
		t.Fatalf("kapat/aç: %+v", fc)
	}
	req := httptest.NewRequest("GET", "/answer/api/v1/hooks/kilavuz/topics/training:abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusFound || !strings.HasSuffix(w.Header().Get("Location"), "/questions/Q1") {
		t.Fatalf("yönlendirme: %d %s", w.Code, w.Header().Get("Location"))
	}
}

func TestRejects(t *testing.T) {
	_, fc, r := setup(t)
	ok := `{"type":"topic.upsert","ref":"a","version":1,"title":"Başlık","content":"x"}`
	cases := map[string]*httptest.ResponseRecorder{
		"yanlış secret":     send(r, "kilavuz", "a1", ok, []byte("yanlis"), now),
		"eski damga":        send(r, "kilavuz", "a2", ok, []byte("gizli"), now.Add(-10*time.Minute)),
		"bilinmeyen kaynak": send(r, "baska", "a3", ok, []byte("gizli"), now),
	}
	want := map[string]int{"yanlış secret": 401, "eski damga": 401, "bilinmeyen kaynak": 404}
	for name, w := range cases {
		if w.Code != want[name] {
			t.Errorf("%s: %d", name, w.Code)
		}
	}
	if fc.created != 0 {
		t.Fatal("reddedilen istek yazmamalı")
	}
	// Yazma hatası: aynı webhook-id ile tekrar denenebilmeli.
	fc.fail = true
	if w := send(r, "kilavuz", "b1", ok, []byte("gizli"), now); w.Code != 422 {
		t.Fatalf("hata: %d", w.Code)
	}
	fc.fail = false
	if w := send(r, "kilavuz", "b1", ok, []byte("gizli"), now); w.Code != 200 || fc.created != 1 {
		t.Fatalf("yeniden deneme: %d", w.Code)
	}
	// Eklenti kapalıyken rota yanıt vermez.
	plugin.StatusManager.Enable(slugName, false)
	if w := send(r, "kilavuz", "c1", ok, []byte("gizli"), now); w.Code != 404 {
		t.Fatalf("kapalı: %d", w.Code)
	}
}

func flat(prefix string, v any, out map[string]bool) {
	if m, ok := v.(map[string]any); ok {
		for k, c := range m {
			flat(prefix+"."+k, c, out)
		}
		return
	}
	out[prefix] = true
}

// Eklentinin her İngilizce anahtarı Türkçede de olmalı.
func TestTurkishKeys(t *testing.T) {
	load := func(f string) map[string]bool {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := yaml.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		flat("", m, out)
		return out
	}
	en, tr := load("i18n/en_US.yaml"), load("i18n/tr_TR.yaml")
	for k := range en {
		if !tr[k] {
			t.Errorf("tr_TR'de eksik: %s", k)
		}
	}
}
