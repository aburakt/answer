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

// ib: intranet girişi. Kullanıcılar sicil ve parolayla doğrudan LDAP'tan
// (plugins/ib-ldap) girer; form, connector'ın redirect ucuna doğal POST yapar.
// E-posta/parola girişi yalnız yöneticiler içindir ve /users/admin'dedir.
import React, { FormEvent, useState } from 'react';
import {
  Alert,
  Button,
  Col,
  Container,
  Form,
  InputGroup,
} from 'react-bootstrap';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

import { usePageTags } from '@/hooks';
import { Icon, PluginRender } from '@/components';
import IbLogo from '@/assets/images/ib-logo.svg';
import { PluginType } from '@/utils/pluginKit';
import { useGetStartUseOauthConnector } from '@/plugins/builtin/ThirdPartyConnector/services';

const LDAP_SLUG = 'ldap';
// Görelidir: site adresi ayarı yanlış olsa da form, açıldığı siteye gider.
const LDAP_ACTION = `/answer/api/v1/connector/redirect/${LDAP_SLUG}`;
// Sicil 6 ya da 7 haneli sayıdır.
const SICIL = /^\d{6,7}$/;
const REASONS = [
  'credentials',
  'mail',
  'account',
  'connect',
  'tls',
  'bind',
  'search',
  'config',
  'identity',
  'origin',
  'internal',
  'unknown',
];

const Index: React.FC = () => {
  const { t } = useTranslation('translation', { keyPrefix: 'login' });
  const [searchParams] = useSearchParams();
  const { data: connectors } = useGetStartUseOauthConnector();
  const ldap = connectors?.find((c) => c.link.endsWith(`/login/${LDAP_SLUG}`));
  const reason = searchParams.get('reason') || 'credentials';
  const [invalid, setInvalid] = useState({
    username: '' as '' | 'empty' | 'format',
    password: false,
  });
  const [showPassword, setShowPassword] = useState(false);

  usePageTags({ title: t('login', { keyPrefix: 'page_title' }) });

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    const form = event.currentTarget;
    const sicil = form.username.value.trim();
    let username: '' | 'empty' | 'format' = '';
    if (!sicil) {
      username = 'empty';
    } else if (!SICIL.test(sicil)) {
      username = 'format';
    }
    const next = { username, password: !form.password.value };
    if (next.username || next.password) {
      event.preventDefault();
      setInvalid(next);
    }
  };

  return (
    <Container style={{ paddingTop: '3rem', paddingBottom: '5rem' }}>
      <Col className="mx-auto ib-login card" sm={10} md={7} lg={5} xl={4}>
        <div className="ib-login-brand">
          <img src={IbLogo} alt="" width={104} height={104} />
          <h1 className="ib-login-title">İSTİHBARAT BAŞKANLIĞI</h1>
          <div className="ib-login-sub">FORUM UYGULAMASI</div>
        </div>
        {searchParams.get('error') === LDAP_SLUG && (
          <Alert variant="danger" role="alert">
            {t(
              REASONS.includes(reason)
                ? `ldap.reason.${reason}`
                : 'ldap.reason.credentials',
            )}
          </Alert>
        )}
        {ldap ? (
          <Form
            noValidate
            method="post"
            action={LDAP_ACTION}
            onSubmit={handleSubmit}>
            <Form.Group controlId="username" className="mb-3">
              <Form.Label>{t('ldap.username.label')}</Form.Label>
              <Form.Control
                name="username"
                autoComplete="username"
                autoFocus
                required
                inputMode="numeric"
                pattern="[0-9]{6,7}"
                maxLength={7}
                isInvalid={Boolean(invalid.username)}
                onChange={(event) => {
                  // Yalnız rakam: harf ve boşluk yazılırken atılır.
                  event.target.value = event.target.value.replace(/\D/g, '');
                  setInvalid({ ...invalid, username: '' });
                }}
              />
              <Form.Control.Feedback type="invalid">
                {t(`ldap.username.msg.${invalid.username || 'empty'}`)}
              </Form.Control.Feedback>
            </Form.Group>
            <Form.Group controlId="password" className="mb-3">
              <Form.Label>{t('password.label')}</Form.Label>
              <InputGroup hasValidation>
                <Form.Control
                  name="password"
                  type={showPassword ? 'text' : 'password'}
                  autoComplete="current-password"
                  required
                  isInvalid={invalid.password}
                  onChange={() => setInvalid({ ...invalid, password: false })}
                />
                <Button
                  variant="outline-secondary"
                  className="ib-password-toggle"
                  aria-label={t(
                    showPassword ? 'ldap.password.hide' : 'ldap.password.show',
                  )}
                  aria-pressed={showPassword}
                  title={t(
                    showPassword ? 'ldap.password.hide' : 'ldap.password.show',
                  )}
                  onClick={() => setShowPassword(!showPassword)}>
                  <Icon name={showPassword ? 'eye-slash' : 'eye'} />
                </Button>
                <Form.Control.Feedback type="invalid">
                  {t('password.msg.empty')}
                </Form.Control.Feedback>
              </InputGroup>
            </Form.Group>
            <div className="d-grid">
              <Button variant="primary" type="submit">
                {t('login', { keyPrefix: 'btns' })}
              </Button>
            </div>
          </Form>
        ) : (
          <>
            <PluginRender
              type={PluginType.Connector}
              slug_name="third_party_connector"
              className="mb-5"
            />
            {connectors && connectors.length === 0 && (
              <Alert variant="info">{t('ldap.unavailable')}</Alert>
            )}
          </>
        )}
      </Col>
    </Container>
  );
};

export default React.memo(Index);
