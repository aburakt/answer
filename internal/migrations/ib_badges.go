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

package migrations

import (
	"fmt"

	"github.com/apache/answer/internal/entity"
	"xorm.io/xorm"
)

// ibBadgeIcons lists the default badges whose icon differs from upstream.
var ibBadgeIcons = map[string]bool{
	"badge.default_badges.autobiographer.name": true,
	"badge.default_badges.scholar.name":        true,
}

// syncIbBadgeIcons copies the icons of those badges from defaultBadgeTable to a
// database that was installed before the fork changed them. Upstream migrations
// never touch the icon of an existing badge. It is idempotent and never fails
// the upgrade.
func syncIbBadgeIcons(x *xorm.Engine) {
	for _, badge := range defaultBadgeTable {
		if !ibBadgeIcons[badge.Name] {
			continue
		}
		n, err := x.Where("name = ? AND icon <> ?", badge.Name, badge.Icon).
			Cols("icon").Update(&entity.Badge{Icon: badge.Icon})
		if err != nil {
			fmt.Printf("[migrate] ib: badge icon %s not updated: %s\n", badge.Name, err.Error())
			continue
		}
		if n > 0 {
			fmt.Printf("[migrate] ib: badge icon %s set to %s\n", badge.Name, badge.Icon)
		}
	}
}
