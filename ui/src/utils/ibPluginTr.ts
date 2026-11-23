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

// ib: Ön yüz eklentileri (yerleşik ve answer-plugins) yalnız en_US ve zh_CN
// taşır. Kullandıklarımızın Türkçesi burada `plugin` namespace'ine eklenir.
import { initI18nResource } from '@/utils/pluginKit/utils';

initI18nResource({
  tr_TR: {
    plugin: {
      third_party_connector: {
        ui: {
          connect: '{{ auth_name }} ile bağlan',
          remove: '{{ auth_name }} bağlantısını kaldır',
        },
      },
      hosting_connector: {
        ui: {
          connect: '{{ auth_name }} ile bağlan',
          login: 'Giriş yap',
          qrcode_login_tip:
            'Giriş yapmak için QR kodu {{ agentName }} ile okutun.',
          login_failed_email_tip:
            'Giriş başarısız. Bu uygulamanın e-posta bilginize erişmesine izin verip tekrar deneyin.',
        },
      },
      serarch_info: {
        ui: {
          search_by: 'Arama sağlayıcısı:',
        },
      },
      basic_captcha: {
        frontend: {
          title: 'Doğrulama kodu',
          placeholder: 'Yukarıdaki metni yazın',
          msg: { empty: 'Doğrulama kodu boş olamaz.' },
          verify: 'Doğrula',
        },
      },
      quick_links: {
        frontend: {
          quick_links: 'Hızlı bağlantılar',
        },
      },
    },
  },
} as any);
