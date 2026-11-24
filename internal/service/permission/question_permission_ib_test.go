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

package permission

import (
	"context"
	"testing"

	"github.com/apache/answer/internal/schema"
	"github.com/stretchr/testify/assert"
)

func names(actions []*schema.PermissionMemberAction) []string {
	out := make([]string, 0, len(actions))
	for _, a := range actions {
		out = append(out, a.Action)
	}
	return out
}

// ib: whoever may pin may announce; an announcement is ended with "unannounce".
func TestWithAnnouncementActions(t *testing.T) {
	act := func(list ...string) []*schema.PermissionMemberAction {
		out := make([]*schema.PermissionMemberAction, 0, len(list))
		for _, a := range list {
			out = append(out, &schema.PermissionMemberAction{Action: a})
		}
		return out
	}
	ctx := context.Background()

	assert.Equal(t, []string{"report", "close"},
		names(WithAnnouncementActions(ctx, act("report", "close"), false)), "no pin permission, no announce")
	assert.Equal(t, []string{"close", "pin", "announce", "hide"},
		names(WithAnnouncementActions(ctx, act("close", "pin", "hide"), false)))
	assert.Equal(t, []string{"unpin", "announce"},
		names(WithAnnouncementActions(ctx, act("unpin"), false)), "pinned question can become an announcement")
	assert.Equal(t, []string{"close", "unannounce"},
		names(WithAnnouncementActions(ctx, act("close", "unpin"), true)))
}
