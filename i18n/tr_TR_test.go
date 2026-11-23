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

package i18n

import (
	"fmt"
	"os"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"
)

// ib: Türkçe çeviri kapısı. en_US'teki her anahtar tr_TR'de bulunmalı ve
// İngilizce metnin kopyası olmamalı. Bilerek aynı kalanlar (teknik terim,
// örnek değer, noktalama) aşağıdaki listededir. Upstream yeni anahtar
// eklediğinde bu test kırılır; çeviri eklenmeden upgrade tamamlanmaz.
var trSameAsEnglishAllowed = map[string]bool{
	"ui.admin.ai_settings.model.label":        true,
	"ui.admin.branding.favicon.label":         true,
	"ui.admin.branding.logo.label":            true,
	"ui.admin.css_and_html.head.label":        true,
	"ui.admin.dashboard.blog":                 true,
	"ui.admin.dashboard.forum":                true,
	"ui.admin.dashboard.github":               true,
	"ui.admin.dashboard.https":                true,
	"ui.admin.dashboard.smtp":                 true,
	"ui.admin.mcp.url.label":                  true,
	"ui.admin.seo.page_title":                 true,
	"ui.admin.seo.robots.label":               true,
	"ui.admin.smtp.encryption.ssl":            true,
	"ui.admin.smtp.encryption.tls":            true,
	"ui.admin.smtp.page_title":                true,
	"ui.admin.users.normal":                   true,
	"ui.btns.normal":                          true,
	"ui.editor.link.form.fields.url.label":    true,
	"ui.install.db_file.placeholder":          true,
	"ui.install.db_host.placeholder":          true,
	"ui.install.db_name.placeholder":          true,
	"ui.install.db_password.placeholder":      true,
	"ui.install.db_username.placeholder":      true,
	"ui.nav_menus.mcp":                        true,
	"ui.nav_menus.seo":                        true,
	"ui.nav_menus.smtp":                       true,
	"ui.personal.comma":                       true,
	"ui.personal.mod_short":                   true,
	"ui.settings.profile.avatar.gravatar":     true,
	"ui.settings.profile.website.placeholder": true,
}

func flatten(prefix string, v any, out map[string]string) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			flatten(key, child, out)
		}
	default:
		out[prefix] = fmt.Sprint(t)
	}
}

func load(t *testing.T, name string) map[string]string {
	t.Helper()
	b, err := I18n.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(b, &raw); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	out := map[string]string{}
	flatten("", raw, out)
	return out
}

func TestTurkishComplete(t *testing.T) {
	en, tr := load(t, "en_US.yaml"), load(t, "tr_TR.yaml")
	var missing, same []string
	for k, v := range en {
		got, ok := tr[k]
		switch {
		case !ok:
			missing = append(missing, k)
		case got == v && !trSameAsEnglishAllowed[k]:
			same = append(same, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(same)
	for _, k := range missing {
		t.Errorf("tr_TR'de eksik: %s", k)
	}
	for _, k := range same {
		t.Errorf("tr_TR'de çevrilmemiş (İngilizceyle aynı): %s = %q", k, en[k])
	}
}

// ib: tr_TR, i18n.yaml'daki dil listesinde olmazsa sunucu onu hiç sunmaz ve
// her şey İngilizce gelir (Accept-Language eşleşmesi bu listeye bakar).
func TestTurkishIsLanguageOption(t *testing.T) {
	buf, err := os.ReadFile("i18n.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Options []struct {
			Value string `yaml:"value"`
		} `yaml:"language_options"`
	}
	if err := yaml.Unmarshal(buf, &s); err != nil {
		t.Fatal(err)
	}
	for _, o := range s.Options {
		if o.Value == "tr_TR" {
			return
		}
	}
	t.Fatal("i18n.yaml language_options içinde tr_TR yok")
}
