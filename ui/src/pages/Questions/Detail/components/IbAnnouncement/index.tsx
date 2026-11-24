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

import { memo, FC } from 'react';
import { useTranslation } from 'react-i18next';

import dayjs from 'dayjs';

import { Icon } from '@/components';

interface Props {
  time: number;
}

// ib: duyuru yapılan sorunun üstündeki bant.
const Index: FC<Props> = ({ time }) => {
  const { t } = useTranslation('translation', {
    keyPrefix: 'question_detail.announce',
  });
  const { t: td } = useTranslation('translation', { keyPrefix: 'dates' });
  const at = dayjs.unix(time).tz();
  return (
    <div className="ib-announcement mb-4" role="note">
      <div className="ib-announcement-heading">
        <Icon name="megaphone-fill" className="me-2" />
        {t('heading')}
      </div>
      <div className="small">
        {t('date')}{' '}
        <time
          dateTime={at.toISOString()}
          title={at.format(td('long_date_with_time'))}>
          {at.format(td('long_date_with_year'))}
        </time>
      </div>
    </div>
  );
};

export default memo(Index);
