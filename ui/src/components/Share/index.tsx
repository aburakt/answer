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

import { memo, FC, useState } from 'react';
import { Dropdown, OverlayTrigger, Tooltip } from 'react-bootstrap';
import { useTranslation } from 'react-i18next';

import copy from 'copy-to-clipboard';
import classNames from 'classnames';

import { BASE_ORIGIN } from '@/router/alias';
import { loggedUserInfoStore } from '@/stores';
import Icon from '../Icon';

interface IProps {
  type: 'answer' | 'question';
  qid: any;
  aid?: any;
  title: string;
  className?: string;
  mode?: 'normal' | 'mobile';
}

// ib: intranette sosyal medya paylaşımı yoktur; "Paylaş" menüsünün yerinde yalnız
// bağlantıyı panoya kopyalayan düğme durur.
const Index: FC<IProps> = ({ type, qid, aid, title, className, mode }) => {
  const user = loggedUserInfoStore((state) => state.user);
  const [showTip, setShowTip] = useState(false);
  const { t } = useTranslation();
  let url =
    type === 'question'
      ? `${BASE_ORIGIN}/questions/${qid}`
      : `${BASE_ORIGIN}/questions/${qid}/${aid}`;
  if (user.id) {
    url = `${url}?share=${user.username}`;
  }

  const handleCopy = () => {
    copy(url);
    setShowTip(true);
    setTimeout(() => setShowTip(false), 1500);
  };

  if (mode === 'mobile') {
    return (
      <Dropdown.Item onClick={handleCopy}>
        <Icon name="link-45deg" className="me-1" />
        {t('share.copy')}
      </Dropdown.Item>
    );
  }
  return (
    <OverlayTrigger
      placement="top"
      show={showTip}
      overlay={<Tooltip>{t('share.copied')}</Tooltip>}>
      <button
        type="button"
        className={classNames(
          'btn btn-link p-0 text-decoration-none d-flex align-items-center',
          className,
        )}
        style={{ lineHeight: '23px' }}
        aria-label={`${t('share.copy')}: ${title}`}
        onClick={handleCopy}>
        <Icon name="link-45deg" className="me-1" />
        {t('share.copy')}
      </button>
    </OverlayTrigger>
  );
};

export default memo(Index);
