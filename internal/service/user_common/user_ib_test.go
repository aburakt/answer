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

package usercommon

import (
	"strings"
	"testing"

	"github.com/apache/answer/pkg/checker"
	"github.com/stretchr/testify/assert"
)

// ib: a display name with Turkish letters must give a valid username.
func TestTurkishDisplayNameGivesValidUsername(t *testing.T) {
	for name, want := range map[string]string{
		"Kılavuz":      "kilavuz",
		"Çağrı Öztürk": "cagri-ozturk",
		"İSMAİL ŞAHİN": "ismail-sahin",
		"Gülşen Ünlü":  "gulsen-unlu",
		"plain name":   "plain-name",
	} {
		got := strings.ToLower(strings.ReplaceAll(turkishToASCII.Replace(name), " ", "-"))
		assert.Equal(t, want, got)
		assert.False(t, checker.IsInvalidUsername(got), got)
	}
}
