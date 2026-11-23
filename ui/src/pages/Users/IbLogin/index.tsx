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
// (connector-ldap) girer; form, connector'ın redirect ucuna doğal POST yapar.
// E-posta/parola girişi yalnız yöneticiler içindir ve /users/admin'dedir.
import React, { FormEvent, useState } from 'react';
import { Alert, Button, Col, Container, Form } from 'react-bootstrap';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

import { usePageTags } from '@/hooks';
import { PluginRender, WelcomeTitle } from '@/components';
import { PluginType } from '@/utils/pluginKit';
import { useGetStartUseOauthConnector } from '@/plugins/builtin/ThirdPartyConnector/services';

const LDAP_SLUG = 'ldap';

const Index: React.FC = () => {
  const { t } = useTranslation('translation', { keyPrefix: 'login' });
  const [searchParams] = useSearchParams();
  const { data: connectors } = useGetStartUseOauthConnector();
  const ldap = connectors?.find((c) => c.link.endsWith(`/login/${LDAP_SLUG}`));
  const [invalid, setInvalid] = useState({ username: false, password: false });

  usePageTags({ title: t('login', { keyPrefix: 'page_title' }) });

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    const form = event.currentTarget;
    const next = {
      username: !form.username.value.trim(),
      password: !form.password.value,
    };
    if (next.username || next.password) {
      event.preventDefault();
      setInvalid(next);
    }
  };

  return (
    <Container style={{ paddingTop: '4rem', paddingBottom: '5rem' }}>
      <WelcomeTitle />
      <Col className="mx-auto" md={6} lg={4} xl={3}>
        {searchParams.get('error') === LDAP_SLUG && (
          <Alert variant="danger" role="alert">
            {t('ldap.error')}
          </Alert>
        )}
        {ldap ? (
          <Form
            noValidate
            method="post"
            action={ldap.link.replace(
              `/connector/login/${LDAP_SLUG}`,
              `/connector/redirect/${LDAP_SLUG}`,
            )}
            onSubmit={handleSubmit}>
            <Form.Group controlId="username" className="mb-3">
              <Form.Label>{t('ldap.username.label')}</Form.Label>
              <Form.Control
                name="username"
                autoComplete="username"
                autoFocus
                required
                isInvalid={invalid.username}
                onChange={() => setInvalid({ ...invalid, username: false })}
              />
              <Form.Control.Feedback type="invalid">
                {t('ldap.username.msg.empty')}
              </Form.Control.Feedback>
            </Form.Group>
            <Form.Group controlId="password" className="mb-3">
              <Form.Label>{t('password.label')}</Form.Label>
              <Form.Control
                name="password"
                type="password"
                autoComplete="current-password"
                required
                isInvalid={invalid.password}
                onChange={() => setInvalid({ ...invalid, password: false })}
              />
              <Form.Control.Feedback type="invalid">
                {t('password.msg.empty')}
              </Form.Control.Feedback>
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
