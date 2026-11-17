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

package importer

// ib: fork yaması, bkz. plugin/content_writer.go.

import (
	"context"
	"fmt"

	"github.com/apache/answer/internal/entity"
	"github.com/apache/answer/internal/schema"
	"github.com/apache/answer/internal/service/permission"
	"github.com/apache/answer/pkg/converter"
	"github.com/apache/answer/pkg/uid"
	"github.com/apache/answer/plugin"
)

// closeReasonSomethingID init_data'daki "reason.something" kaydı (ID sabit).
const closeReasonSomethingID = 59

type contentFunc struct {
	ip *ImporterService
}

// NewContentFunc eklentilere verilen yazma işlemleri.
func (ip *ImporterService) NewContentFunc() plugin.ContentFunc {
	return &contentFunc{ip: ip}
}

func (f *contentFunc) user(ctx context.Context, email string) (*entity.User, error) {
	u, exist, err := f.ip.userCommon.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if !exist || u.Status != entity.UserStatusAvailable {
		return nil, fmt.Errorf("kullanıcı bulunamadı ya da etkin değil: %s", email)
	}
	return u, nil
}

func tagItems(tags []string) []*schema.TagItem {
	out := make([]*schema.TagItem, 0, len(tags))
	for _, t := range tags {
		out = append(out, &schema.TagItem{SlugName: t, DisplayName: t})
	}
	return out
}

func (f *contentFunc) CreateQuestion(ctx context.Context, info plugin.QuestionWriteInfo) (string, error) {
	u, err := f.user(ctx, info.UserEmail)
	if err != nil {
		return "", err
	}
	req := &schema.QuestionAdd{
		Title:   info.Title,
		Content: info.Content,
		HTML:    converter.Markdown2HTML(info.Content),
		Tags:    tagItems(info.Tags),
		UserID:  u.ID,
	}
	can, _, err := f.ip.rankService.CheckOperationPermissionsForRanks(ctx, u.ID, []string{
		permission.QuestionAdd, permission.TagUseReservedTag, permission.TagAdd,
	})
	if err != nil {
		return "", err
	}
	if !can[0] {
		return "", fmt.Errorf("kullanıcının soru açma yetkisi yok")
	}
	req.CanAdd, req.CanEdit, req.CanClose, req.CanReopen = true, true, true, true
	req.CanUseReservedTag, req.CanAddTag = can[1], can[2]
	if _, err := f.ip.questionService.CheckAddQuestion(ctx, req); err != nil {
		return "", err
	}
	resp, err := f.ip.questionService.AddQuestion(ctx, req)
	if err != nil {
		return "", err
	}
	q, ok := resp.(*schema.QuestionInfoResp)
	if !ok || q.ID == "" {
		return "", fmt.Errorf("soru açıldı ama ID alınamadı")
	}
	return q.ID, nil
}

// owned soruyu okur ve sahibinin u olduğunu doğrular.
func (f *contentFunc) owned(ctx context.Context, questionID string, u *entity.User) error {
	q, err := f.ip.questionService.GetQuestion(ctx, questionID, u.ID, schema.QuestionPermission{})
	if err != nil {
		return err
	}
	if q.UserInfo == nil || uid.DeShortID(q.UserInfo.ID) != uid.DeShortID(u.ID) {
		return fmt.Errorf("soru bu kullanıcıya ait değil")
	}
	return nil
}

func (f *contentFunc) UpdateQuestion(ctx context.Context, questionID string, info plugin.QuestionWriteInfo) error {
	u, err := f.user(ctx, info.UserEmail)
	if err != nil {
		return err
	}
	if err := f.owned(ctx, questionID, u); err != nil {
		return err
	}
	can, _, err := f.ip.rankService.CheckOperationPermissionsForRanks(ctx, u.ID, []string{
		permission.TagUseReservedTag, permission.TagAdd,
	})
	if err != nil {
		return err
	}
	req := &schema.QuestionUpdate{
		ID:          questionID,
		Title:       info.Title,
		Content:     info.Content,
		HTML:        converter.Markdown2HTML(info.Content),
		Tags:        tagItems(info.Tags),
		EditSummary: info.EditSummary,
		UserID:      u.ID,
		// Sahibi güncellediği için inceleme gerekmez (controller'daki objectOwner kuralı).
		NoNeedReview: true,
	}
	req.CanEdit = true
	req.CanUseReservedTag, req.CanAddTag = can[0], can[1]
	_, err = f.ip.questionService.UpdateQuestion(ctx, req)
	return err
}

func (f *contentFunc) CloseQuestion(ctx context.Context, questionID, userEmail, message string) error {
	u, err := f.user(ctx, userEmail)
	if err != nil {
		return err
	}
	if err := f.owned(ctx, questionID, u); err != nil {
		return err
	}
	return f.ip.questionService.CloseQuestion(ctx, &schema.CloseQuestionReq{
		ID: questionID, CloseType: closeReasonSomethingID, CloseMsg: message, UserID: u.ID,
	})
}

func (f *contentFunc) ReopenQuestion(ctx context.Context, questionID, userEmail string) error {
	u, err := f.user(ctx, userEmail)
	if err != nil {
		return err
	}
	if err := f.owned(ctx, questionID, u); err != nil {
		return err
	}
	return f.ip.questionService.ReopenQuestion(ctx, &schema.ReopenQuestionReq{QuestionID: questionID, UserID: u.ID})
}
